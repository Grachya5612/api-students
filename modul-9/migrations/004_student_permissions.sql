BEGIN;

INSERT INTO permissions (name, description)
VALUES
    ('student:list', 'Melihat daftar data student'),
    ('student:read:any', 'Melihat data student milik siapa pun'),
    ('student:create', 'Membuat data student'),
    ('student:update:any', 'Mengubah data student milik siapa pun'),
    ('student:delete', 'Menghapus data student')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name)
VALUES
    ('admin', 'student:list'),
    ('staff', 'student:list'),

    ('admin', 'student:read:any'),
    ('staff', 'student:read:any'),

    ('admin', 'student:create'),

    ('admin', 'student:update:any'),

    ('admin', 'student:delete')
ON CONFLICT DO NOTHING;

ALTER TABLE students
ADD COLUMN IF NOT EXISTS owner_id INTEGER;

ALTER TABLE students
ADD CONSTRAINT fk_students_owner
FOREIGN KEY (owner_id)
REFERENCES users(id)
ON DELETE SET NULL;


COMMIT;