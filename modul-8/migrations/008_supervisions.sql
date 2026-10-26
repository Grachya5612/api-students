CREATE TABLE IF NOT EXISTS supervisors (
 id SERIAL PRIMARY KEY,
 name VARCHAR(120) NOT NULL,
 quota INTEGER NOT NULL CHECK (quota > 0),
 supervised_count INTEGER NOT NULL DEFAULT 0,
 version INTEGER NOT NULL DEFAULT 1,
 -- Jaring pengaman. Bila kode benar, constraint ini tidak pernah terpicu.
 CONSTRAINT supervisors_count_within_quota
 CHECK (supervised_count >= 0 AND supervised_count <= quota)
);

-- PRIMARY KEY pada student_id adalah constraint "satu mahasiswa, satu pembimbing".
-- Mahasiswa yang sama tidak mungkin punya dua baris, sepintar apa pun urutan request.
CREATE TABLE IF NOT EXISTS supervisions (
 student_id INTEGER NOT NULL PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
 supervisor_id INTEGER NOT NULL REFERENCES supervisors(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS supervisions_supervisor_idx ON supervisions (supervisor_id);

INSERT INTO supervisors (name, quota) VALUES
 ('Dr. Budi Santoso', 3),
 ('Dr. Siti Rahayu', 3),
 ('Dr. Agus Wijaya', 5)
ON CONFLICT DO NOTHING;

INSERT INTO permissions (name, description) VALUES
 ('supervision:manage', 'Mengelola pembimbing dan penetapan mahasiswa')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name) VALUES
 ('admin', 'supervision:manage')
ON CONFLICT DO NOTHING;