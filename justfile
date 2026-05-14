test:
    go test ./...

coverage:
    scripts/check-coverage.sh

dev *args:
    rm -f ./openminutes
    go build -o ./openminutes .
    ./openminutes {{args}}

run *args:
    rm -f ./openminutes
    go build -o ./openminutes .
    ./openminutes --config ./test.config.toml {{args}}
