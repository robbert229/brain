run:
	go run ./cmd/brain

test:
	go test ./...

refresh-tasknotes-fixtures:
	rm -rf ./internal/tn/tasknotes
	rm -rf ./internal/tn/testdata/tasknotes-e2e-vault
	mkdir -p ./internal/tn/testdata
	git clone https://github.com/callumalpass/tasknotes.git ./internal/tn/tasknotes
	mv ./internal/tn/tasknotes/tasknotes-e2e-vault ./internal/tn/testdata/