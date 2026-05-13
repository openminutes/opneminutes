dev *args:
    rm -f ./openminutes
    go build -o ./openminutes .
    ./openminutes {{args}}
