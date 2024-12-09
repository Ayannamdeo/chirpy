-- +goose Up
ALTER TABLE users
add column is_chirpy_red boolean not null default false;

-- +goose Down
ALTER TABLE users
drop column is_chirpy_red;
