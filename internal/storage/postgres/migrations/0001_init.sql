-- +goose Up
CREATE TABLE tasks (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id     BIGINT NOT NULL,
    title       TEXT NOT NULL CHECK (btrim(title) <> ''),
    creator_id  BIGINT NOT NULL,
    creator     TEXT NOT NULL,
    assignee_id BIGINT,
    assignee    TEXT NOT NULL DEFAULT '',
    done        BOOLEAN NOT NULL DEFAULT FALSE
);

-- Список открытых задач чата: частичный индекс дешевле полного.
CREATE INDEX tasks_chat_open_idx ON tasks (chat_id, id) WHERE NOT done;

-- «Мои задачи»: ищем по исполнителю внутри чата.
CREATE INDEX tasks_assignee_idx ON tasks (chat_id, assignee_id) WHERE assignee_id IS NOT NULL;

-- «Созданные мной».
CREATE INDEX tasks_creator_idx ON tasks (chat_id, creator_id);

-- +goose Down
DROP TABLE tasks;
