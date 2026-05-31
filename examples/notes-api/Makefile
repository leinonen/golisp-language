BINARY := notes-api
DB_URL  := postgres://notes:notes@localhost:5433/notes
PORT    := 4000

.PHONY: build run up down docker-up docker-down logs clean

build:
	cd ../.. && ./glisp build -o examples/notes-api/$(BINARY) examples/notes-api/

run: up build
	DATABASE_URL=$(DB_URL) PORT=$(PORT) ./$(BINARY)

up:
	docker compose up -d postgres

down:
	docker compose down

docker-up: build
	docker compose up -d --build

docker-down:
	docker compose down -v

logs:
	docker compose logs -f api

clean:
	rm -f $(BINARY)
	docker compose down -v
