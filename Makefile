BINARY := dtrack-submit
DIST   := dist

.PHONY: build build-all clean

build:
	go build -o $(BINARY)$(if $(filter windows,$(GOOS)),.exe,) .

build-all:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/$(BINARY)-windows-amd64.exe .
	GOOS=linux   GOARCH=amd64 go build -o $(DIST)/$(BINARY)-linux-amd64   .
	GOOS=darwin  GOARCH=amd64 go build -o $(DIST)/$(BINARY)-darwin-amd64  .
	GOOS=darwin  GOARCH=arm64 go build -o $(DIST)/$(BINARY)-darwin-arm64  .

clean:
	rm -rf $(DIST) $(BINARY) $(BINARY).exe
