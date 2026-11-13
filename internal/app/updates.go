package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	releasesAPI  = "https://api.github.com/repos/Sage9203/mygo-cloudmusic/releases"
	releasesPage = "https://github.com/Sage9203/mygo-cloudmusic/releases/tag/"
)

// Match the tags used by this project, and allow a local build suffix.
var releaseVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-preview\.(0|[1-9][0-9]*))?(?:\+[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)*)?$`)
var releaseHeading = regexp.MustCompile(`^#{1,6}[ \t]+`)

// Compare major, minor, patch, stable flag, then preview number.
type releaseVersion [5]uint64

func parseReleaseVersion(s string) (releaseVersion, bool) {
	var version releaseVersion
	match := releaseVersionPattern.FindStringSubmatch(s)
	if match == nil {
		return version, false
	}
	for i, part := range match[1:] {
		if part == "" {
			version[3] = 1
			break
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, false
		}
		if i == 3 {
			version[4] = n
		} else {
			version[i] = n
		}
	}
	return version, true
}

type appRelease struct {
	Tag         string    `json:"tag_name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
}

func (r appRelease) notes() string {
	lines := strings.Split(strings.TrimSpace(r.Body), "\n")
	for i, line := range lines {
		line = releaseHeading.ReplaceAllString(strings.TrimSpace(line), "")
		if item, ok := strings.CutPrefix(line, "- "); ok {
			line = "• " + item
		}
		lines[i] = line
	}
	notes := []rune(strings.Join(lines, "\n"))
	const limit = 600
	if len(notes) > limit {
		summary := string(notes[:limit])
		if lineEnd := strings.LastIndexByte(summary, '\n'); lineEnd >= len(string(notes[:limit/2])) {
			summary = summary[:lineEnd]
		}
		return strings.TrimSpace(summary) + "…"
	}
	return string(notes)
}

type releaseCheckState struct {
	running bool
	done    bool
	newer   bool
	release appRelease
	err     error
}

func (s releaseCheckState) message() string {
	var networkError net.Error
	switch {
	case s.running:
		return "正在查询 GitHub 发布版本…"
	case errors.Is(s.err, context.DeadlineExceeded), errors.As(s.err, &networkError) && networkError.Timeout():
		return "连接 GitHub 超时，请稍后重试"
	case s.err != nil:
		return "暂时无法检查更新，请稍后重试"
	case s.newer:
		return "发现新版本"
	case s.done:
		return "当前没有可用更新"
	default:
		return "手动查询 GitHub 发布版本"
	}
}

var checkAppRelease = func(current string) (appRelease, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return findAppRelease(ctx, client, current)
}

func (a *app) startReleaseCheck(current string, post func(func())) {
	if a.releaseCheck.running {
		return
	}
	a.releaseCheck = releaseCheckState{running: true}
	check := checkAppRelease
	go func() {
		release, newer, err := check(current)
		post(func() {
			a.releaseCheck = releaseCheckState{done: true, newer: newer, release: release, err: err}
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
		})
	}()
}

func findAppRelease(ctx context.Context, client *http.Client, current string) (appRelease, bool, error) {
	version, ok := parseReleaseVersion(current)
	if !ok {
		return appRelease{}, false, errors.New("invalid app version")
	}
	var latest appRelease
	var latestVersion releaseVersion
	const pageSize = 100
	for page := 1; ; page++ {
		list, err := fetchReleasePage(ctx, client, fmt.Sprintf("%s?per_page=%d&page=%d", releasesAPI, pageSize, page))
		if err != nil {
			return appRelease{}, false, err
		}
		for _, release := range list {
			candidate, valid := parseReleaseVersion(release.Tag)
			if !valid || release.Draft || release.PublishedAt.IsZero() {
				continue
			}
			if version[3] == 1 && (release.Prerelease || candidate[3] == 0) {
				continue
			}
			order := slices.Compare(candidate[:], latestVersion[:])
			if latest.Tag == "" || order > 0 || order == 0 && release.PublishedAt.After(latest.PublishedAt) {
				latest, latestVersion = release, candidate
			}
		}
		if len(list) < pageSize {
			break
		}
	}
	if latest.Tag == "" {
		return appRelease{}, false, errors.New("no eligible releases")
	}
	return latest, slices.Compare(latestVersion[:], version[:]) > 0, nil
}

func fetchReleasePage(ctx context.Context, client *http.Client, url string) ([]appRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub releases returned HTTP %d", resp.StatusCode)
	}
	const limit = 4 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errors.New("GitHub release response exceeds size limit")
	}
	var releases []appRelease
	err = json.Unmarshal(data, &releases)
	return releases, err
}
