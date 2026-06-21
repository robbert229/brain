run:
	go run ./cmd/brain

test:
	go test ./...

install-go-jsonschema:
	go install github.com/atombender/go-jsonschema@latest

refresh-tasknotes-fixtures:
	rm -rf ./internal/tnservice/tasknotes
	rm -rf ./internal/tnservice/testdata/tasknotes-e2e-vault
	mkdir -p ./internal/tnservice/testdata
	git clone https://github.com/callumalpass/tasknotes.git ./internal/tnservice/tasknotes
	mv ./internal/tnservice/tasknotes/tasknotes-e2e-vault ./internal/tnservice/testdata/
