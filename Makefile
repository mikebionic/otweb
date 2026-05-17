run:
	go run .

stop:
	kill $$(lsof -ti:5500) 2>/dev/null || true

restart: stop
	sleep 1
	go run . &

build:
	go build -o otapi-hub .

test:
	go test ./...

deploy:
	GOOS=linux GOARCH=amd64 go build -o otapi-hub-linux .
	@echo "Binary: otapi-hub-linux (14MB)"
	@echo "Upload: scp -i ~/.ssh/id_ed25519 otapi-hub-linux root@95.181.224.97:/opt/otapi-hub/otapi-hub"
