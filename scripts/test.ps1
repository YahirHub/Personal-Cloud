$ErrorActionPreference = "Stop"

Write-Host "==> gofmt"
$unformatted = gofmt -l ./cmd ./internal ./web
if ($unformatted) {
    $unformatted | Write-Host
    throw "Hay archivos Go sin formato."
}

Write-Host "==> go test"
go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> go vet"
go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "OK"
