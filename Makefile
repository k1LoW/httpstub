export GO111MODULE=on

default: test

ci: depsdev test

test: cert
	go test ./... -coverprofile=coverage.out -covermode=count

lint:
	golangci-lint run ./...

cert:
	mkdir -p testdata
	rm -f testdata/*.pem testdata/*.srl
	openssl req -x509 -newkey rsa:4096 -days 365 -nodes -sha256 -keyout testdata/cakey.pem -out testdata/cacert.pem -subj "/C=UK/ST=Test State/L=Test Location/O=Test Org/OU=Test Unit/CN=*.example.com/emailAddress=k1lowxb@gmail.com"
	openssl req -newkey rsa:4096 -nodes -keyout testdata/key.pem -out testdata/csr.pem -subj "/C=JP/ST=Test State/L=Test Location/O=Test Org/OU=Test Unit/CN=*.example.com/emailAddress=k1lowxb@gmail.com"
	openssl x509 -req -sha256 -in testdata/csr.pem -days 60 -CA testdata/cacert.pem -CAkey testdata/cakey.pem -CAcreateserial -out testdata/cert.pem -extfile testdata/openssl.cnf
	openssl verify -CAfile testdata/cacert.pem testdata/cert.pem

depsdev:
	go install github.com/Songmu/ghch/cmd/ghch@latest
	go install github.com/Songmu/gocredits/cmd/gocredits@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest

prerelease:
	git pull origin main --tag
	go mod tidy
	ghch -w -N ${VER}
	gocredits -w .
	git add CHANGELOG.md CREDITS go.mod go.sum
	git commit -m'Bump up version number'
	git tag ${VER}

release:
	git push origin main --tag

.PHONY: default test
