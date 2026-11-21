package netease

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

// This is the public key used by the NetEase web client to wrap its per-request AES key.
const webPublicKey = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDgtQn2JZ34ZC28NWYpAUd98iZ37BUrX/aKzmFbt7clFSs6sXqHauqKWqdtLkF2KexO40H1YTX8z2lSgBBOAxLsvaklV8k4cBFK9snQXE9/DDaFt6Rr7iVZMldczhC0JNgTz+SHXT6CBHuX3e9SdB1Ua44oncaTWz7OBGLbCiK45wIDAQAB"

func webSongURL(ctx context.Context, sessionClient *http.Client, cookies []*http.Cookie, id int64, level string, out any) error {
	data := map[string]string{"ids": fmt.Sprintf("[%d]", id), "level": level, "encodeType": "flac", "csrf_token": ""}
	for _, cookie := range cookies {
		if cookie.Name == "__csrf" {
			data["csrf_token"] = cookie.Value
		}
	}
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	form, err := encryptWebRequest(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/weapi/song/enhance/player/url/v1", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	req.Header.Set("Referer", apiBase+"/")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{
		Transport:     sessionClient.Transport,
		Timeout:       sessionClient.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &SongURLError{Code: res.StatusCode, Message: http.StatusText(res.StatusCode)}
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func encryptWebRequest(body []byte) (url.Values, error) {
	der, err := base64.StdEncoding.DecodeString(webPublicKey)
	if err != nil {
		return nil, err
	}
	parsedKey, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	publicKey := parsedKey.(*rsa.PublicKey)
	secret := make([]byte, aes.BlockSize)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := range secret {
		secret[i] = alphabet[int(secret[i])%len(alphabet)]
	}
	params := webAES([]byte(webAES(body, []byte("0CoJUm6Qyw8W8jud"))), secret)
	// The web wire format uses the public RSA operation on the reversed key.
	for i, j := 0, len(secret)-1; i < j; i, j = i+1, j-1 {
		secret[i], secret[j] = secret[j], secret[i]
	}
	key := new(big.Int).SetBytes(secret)
	key.Exp(key, big.NewInt(int64(publicKey.E)), publicKey.N)
	return url.Values{"params": {params}, "encSecKey": {hex.EncodeToString(key.FillBytes(make([]byte, publicKey.Size())))}}, nil
}

func webAES(data, key []byte) string {
	block, _ := aes.NewCipher(key)
	padding := aes.BlockSize - len(data)%aes.BlockSize
	padded := append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
	cipher.NewCBCEncrypter(block, []byte("0102030405060708")).CryptBlocks(padded, padded)
	return base64.StdEncoding.EncodeToString(padded)
}
