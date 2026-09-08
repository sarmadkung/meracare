-- Safe lifecycle controls for managed senior profiles and care circles.

ALTER TABLE senior_profiles
    ADD COLUMN archived_at timestamptz,
    ADD COLUMN archived_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL;

CREATE INDEX senior_profiles_active_idx
    ON senior_profiles (id) WHERE archived_at IS NULL;
