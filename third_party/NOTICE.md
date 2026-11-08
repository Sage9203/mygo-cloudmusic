# Additional component notices

The application is maintained under the GNU AGPL version 3 (see ../LICENSE).
The following components retain their own notices and licenses. This file does not relicense them.

## MyGo / Ebitengine shader compiler

MyGo is pinned to github.com/egoist/mygo v0.8.1.
Its internal/shader package documents a fork of Ebitengine's Kage compiler and internal/shaderir at commit 16f18b0.
Copyright 2020-2023 The Ebiten / Ebitengine Authors, as recorded in the upstream source headers.
These packages and the shaderir backends retain their Apache License 2.0 source headers.
The license text is in mygo-ebitengine/LICENSE. Sources are available through the pinned MyGo module, including the original notices and modification comments.

## Lucide and Feather icon paths

internal/app/icons.go includes SVG paths matching Lucide icons and adapted variants.
A fixed comparison was made against lucide-icons/lucide commit ee8e10d746a10907205366cfd984922f0c58939a.
For example, the five geometric elements of iconQueue match icons/list-music.svg.
The application uses its own stroke width and includes modified or additional artwork; the comparison commit is a provenance reference, not a claim that this entire file was imported at that version.
Lucide and Feather notices, including the upstream list of Feather-derived icons, are preserved in lucide/LICENSE (ISC and MIT notices).
Source: https://github.com/lucide-icons/lucide/tree/ee8e10d746a10907205366cfd984922f0c58939a

## Public Suffix List data

golang.org/x/net v0.59.0 embeds a compiled Public Suffix List snapshot.
Its publicsuffix/table.go identifies data revision d6c92f1bbb7433e5db7b8405c25d4035fb8ff376.
The unmodified source snapshot is provided in publicsuffix/public_suffix_list.dat with its original MPL 2.0 header; license text is in publicsuffix/LICENSE.
Source: https://github.com/publicsuffix/list/blob/d6c92f1bbb7433e5db7b8405c25d4035fb8ff376/public_suffix_list.dat
The Go implementation and generated table retain the Go module's own BSD-style notices.

## Go runtime and standard library

The release is built with Go 1.27.3. The Go Authors' license is in go/LICENSE.
The C compiler and LAME encoder used for validation and test-fixture generation are build-only tools and are not shipped or linked into the normal CGO_ENABLED=0 application.

## Project artwork and synthetic test audio

Project artwork is retained from hyoban/mygo-cloudmusic commit b13f52a2a2ff0746502b85b9c4c7fa1e86a1c385 under that repository's AGPLv3 declaration, except for the separately acknowledged icon paths above. No independent authorship claim is made for upstream artwork.
Synthetic audio fixtures are generated from scripts/testaudio/main.go; see internal/player/testdata/README.md.
## NSIS installer components

MyGo pins NSIS 3.13 and checks the official archive SHA256 ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8. The distribution license and component notices from the cached NSIS release are preserved in nsis/COPYING. The complete compiler is not included in the application package.
