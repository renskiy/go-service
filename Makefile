proto:
	protoc --proto_path=api/proto \
		--go_out=pkg \
		--go-grpc_out=pkg \
		./api/proto/*.proto

tests:
	go test -v -count 1 -race ./...

reset-db:
	migrate -path db/migrations -database "postgres://postgres@localhost/service_test?sslmode=disable" drop -f && migrate -path db/migrations -database "postgres://postgres@localhost/service_test?sslmode=disable" up
