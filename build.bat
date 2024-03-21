@echo off
setlocal

if not exist dist mkdir dist

if "%1"=="" (
  set VERSION=pngConvert
) else (
  set VERSION=%1
)

set GOOS=darwin
set GOARCH=amd64
go build -o dist\%VERSION%-darwin-amd64 .

set GOOS=windows
set GOARCH=amd64
go build -o dist\%VERSION%-windows-amd64.exe .

set GOOS=linux
set GOARCH=amd64
go build -o dist\%VERSION%-linux-amd64 .

endlocal
