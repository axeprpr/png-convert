# png-convert

`png-convert` converts one source PNG into:

- Linux hicolor icons under `icons/hicolor/<size>x<size>/apps/`
- a `pixmaps/<name>.png` file at 128x128
- a Windows `.ico`
- a macOS `.icns`

## Repository

```bash
git clone https://github.com/axeprpr/png-convert.git
cd png-convert
```

## Requirements

- Go 1.21+

## Usage

```bash
go run . \
  -i input.png \
  -o app.png \
  -w app.ico \
  -m AppIcon.icns \
  -d dist \
  -clean
```

## Flags

- `-i`: input PNG path
- `-o`: generated PNG filename inside Linux icon directories
- `-w`: ICO filename
- `-m`: ICNS filename
- `-d`: base output directory
- `-clean`: remove generated `icons/hicolor` and `pixmaps` inside the output directory before writing
- `-sizes`: comma-separated Linux and ICO icon sizes, default `16,24,32,48,64,96,128,256,512`
- `-only`: comma-separated outputs to generate, choose from `linux,pixmap,ico,icns`
- `-fit`: `stretch` to force square resize, `contain` to preserve aspect ratio with padding, `cover` to preserve aspect ratio and crop
- `-background`: background color for `contain`, use `transparent` or hex like `#112233` / `#112233ff`
- `-manifest`: optional JSON filename for recording generated artifacts
- `-archive`: optional `.zip` filename for packaging generated artifacts
- `-version`: print build version and exit

`pixmaps/<name>.png` is always generated at `128x128`, even if `128` is not included in `-sizes`.

## Development

```bash
go test ./...
go build ./...
```

## Release

Push a tag like `v1.0.0` to trigger the GitHub Actions release workflow. It builds:

- Linux `amd64`
- Linux `arm64`
- Windows `amd64`
- Windows `arm64`
- macOS `amd64`
- macOS `arm64`

and uploads the binaries plus `SHA256SUMS.txt` as GitHub release assets. You can also run the workflow manually with `workflow_dispatch`.
