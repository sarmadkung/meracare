-- 0010_notes_and_messages — the two remaining MVP coordination domains.

CREATE TABLE care_notes (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    senior_id       uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,
    author_user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    content         text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT care_notes_content_not_blank CHECK (length(btrim(content)) > 0),
    CONSTRAINT care_notes_content_bounded CHECK (length(content) <= 4000)
);

CREATE INDEX care_notes_senior_timeline_idx
    ON care_notes (senior_id, created_at DESC, id DESC);

CREATE TRIGGER care_notes_set_updated_at
    BEFORE UPDATE ON care_notes
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE messages (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    senior_id       uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,
    sender_user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    content         text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT messages_content_not_blank CHECK (length(btrim(content)) > 0),
    CONSTRAINT messages_content_bounded CHECK (length(content) <= 2000),
    CONSTRAINT messages_senior_id_id_unique UNIQUE (senior_id, id)
);

CREATE INDEX messages_senior_timeline_idx
    ON messages (senior_id, created_at DESC, id DESC);

CREATE TABLE message_read_states (
    senior_id             uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,
    user_id               uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_read_message_id  uuid,
    last_read_at          timestamptz,
    updated_at            timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (senior_id, user_id),
    CONSTRAINT message_read_states_message_in_circle
        FOREIGN KEY (senior_id, last_read_message_id)
        REFERENCES messages (senior_id, id) ON DELETE SET NULL (last_read_message_id),
    CONSTRAINT message_read_states_position_complete
        CHECK ((last_read_message_id IS NULL) = (last_read_at IS NULL))
);

CREATE TRIGGER message_read_states_set_updated_at
    BEFORE UPDATE ON message_read_states
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
