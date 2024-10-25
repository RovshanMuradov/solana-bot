POSTGRESQL_URL='postgres://rovshan:muradov25@db:5432/ton_wallet?sslmode=disable'
MIGRATIONS_DIR=./migrations/migrations

.PHONY: up down migrate-up migrate-down migrate-create

up:
	docker-compose up -d

down:
	docker-compose down

migrate-up:
	docker-compose run --rm app migrate -database "${POSTGRESQL_URL}" -path /migrations up

migrate-down:
	docker-compose run --rm app migrate -database "${POSTGRESQL_URL}" -path /migrations down 1

migrate-create:
	@read -p "Enter migration name: " name; \
	migrate create -ext sql -dir ${MIGRATIONS_DIR} -seq $$name; \
	echo "Migration files created in ${MIGRATIONS_DIR}"