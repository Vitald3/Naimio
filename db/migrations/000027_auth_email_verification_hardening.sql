DROP INDEX IF EXISTS users_email_unique;
CREATE UNIQUE INDEX users_email_unique ON users(email_normalized);
