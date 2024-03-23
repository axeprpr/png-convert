# pngConvert

`pngConvert` converts one source PNG into:

- Linux hicolor icons under `icons/hicolor/<size>x<size>/apps/`
- a `pixmaps/<name>.png` file at 128x128
- a Windows `.ico`
- a macOS `.icns`

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
- `-sizes`: comma-separated icon sizes, default `16,24,32,48,64,96,128,256,512`
- `-version`: print build version and exit

## Development

```bash
go test ./...
go build ./...
```

## Release

Push a tag like `v1.0.0` to trigger the GitHub Actions release workflow. It builds:

- Linux `amd64`
- Windows `amd64`
- macOS `amd64`

and uploads the binaries as GitHub release assets.
