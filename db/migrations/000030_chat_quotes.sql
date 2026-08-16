ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_quote text CHECK(reply_quote IS NULL OR char_length(reply_quote)<=1000);
