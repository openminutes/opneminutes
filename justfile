test:
    go test ./...

coverage:
    scripts/check-coverage.sh

dev *args:
    rm -f ./openminutes
    go build -o ./openminutes .
    ./openminutes {{args}}
