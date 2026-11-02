CREATE INDEX IF NOT EXISTS students_cursor_pagination_idx
    ON students (created_at DESC, id DESC);
