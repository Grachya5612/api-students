INSERT INTO users (username, email, password, role)
SELECT 'mhs' || LPAD(g::TEXT, 3, '0'),
       'mhs' || LPAD(g::TEXT, 3, '0') || '@student.unair.ac.id',
       '!', 'user'
FROM generate_series(1, 60) AS g
ON CONFLICT DO NOTHING;