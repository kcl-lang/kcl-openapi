rm -rf $PROJECT_ROOT/_build/bin
mkdir -p $PROJECT_ROOT/_build/bin
go build -o $PROJECT_ROOT/_build/bin/kclopenapi $PROJECT_ROOT
