.PHONY: all deps test build cover

GO ?= go

all: build

deps:
	${GO} get

build: deps
	${GO} build -ldflags "-s -w"

test: deps
	${GO} test -v

clean:
	@rm -rf poormanscdn *.out

cover:
	${GO} test -cover && \
	${GO} test -coverprofile=coverage.out  && \
	${GO} tool cover -html=coverage.out
