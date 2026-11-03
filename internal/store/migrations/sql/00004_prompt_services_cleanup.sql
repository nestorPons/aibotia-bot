-- +goose Up
-- prompt_services_list deja de ser un prompt RAG: ahora es la tabla services
-- más el prompt de notas. Se elimina el embedding legado si existía.
DELETE FROM embeddings WHERE code = 'prompt_services_list';

-- +goose Down
-- No-op: el embedding se regenera con `make embed` si hiciera falta.
