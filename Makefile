all: wtftpd

test:
	go test ./...

wtftpd:
	go build -o wtftpd cmd/wtftpd/wtftpd.go

clean:
	rm -f ./wtftpd

