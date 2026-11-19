package storage

import (
 "context"
 "database/sql"
 "time"
)

type TelegramSettings struct {
 Enabled bool
 NotifyDown bool
 NotifyUp bool
 ChatID string
 EncryptedToken string
}
type TelegramMessage struct {
 ID int64
 Text string
 Attempts int
}
func (d *DB) MigrateTelegram() error {
 _, err := d.Exec(`
 CREATE TABLE IF NOT EXISTS telegram_settings (
  id INTEGER PRIMARY KEY CHECK(id=1),
  enabled INTEGER NOT NULL DEFAULT 0,
  notify_down INTEGER NOT NULL DEFAULT 1,
  notify_up INTEGER NOT NULL DEFAULT 1,
  chat_id TEXT NOT NULL DEFAULT '',
  token_ciphertext TEXT NOT NULL DEFAULT ''
 );
 INSERT OR IGNORE INTO telegram_settings(id) VALUES(1);
 CREATE TABLE IF NOT EXISTS telegram_outbox (
  id INTEGER PRIMARY KEY,
  message TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_telegram_outbox_due ON telegram_outbox(next_attempt_at);
 `)
 return err
}
func (d *DB) GetTelegram() (TelegramSettings,error) {
 var s TelegramSettings
 var enabled, down, up int
 err := d.QueryRow(`SELECT enabled,notify_down,notify_up,chat_id,token_ciphertext FROM telegram_settings WHERE id=1`).Scan(&enabled,&down,&up,&s.ChatID,&s.EncryptedToken)
 s.Enabled=enabled==1; s.NotifyDown=down==1; s.NotifyUp=up==1
 return s,err
}
func (d *DB) SaveTelegram(s TelegramSettings) error {
 _,err:=d.Exec(`UPDATE telegram_settings SET enabled=?,notify_down=?,notify_up=?,chat_id=?,token_ciphertext=? WHERE id=1`,
 s.Enabled,s.NotifyDown,s.NotifyUp,s.ChatID,s.EncryptedToken)
 return err
}
func (d *DB) NextTelegramMessage(ctx context.Context) (TelegramMessage,error) {
 var m TelegramMessage
 err:=d.QueryRowContext(ctx,`SELECT id,message,attempts FROM telegram_outbox WHERE next_attempt_at<=? ORDER BY id LIMIT 1`,time.Now().UTC().Format(time.RFC3339)).Scan(&m.ID,&m.Text,&m.Attempts)
 return m,err
}
func (d *DB) MarkTelegramSent(ctx context.Context,id int64) error {
 _,err:=d.ExecContext(ctx,`DELETE FROM telegram_outbox WHERE id=?`,id)
 return err
}
func (d *DB) RetryTelegram(ctx context.Context,id int64,attempts int) error {
 delay:=time.Duration(1<<min(attempts,8))*time.Second
 if delay>15*time.Minute {delay=15*time.Minute}
 _,err:=d.ExecContext(ctx,`UPDATE telegram_outbox SET attempts=?,next_attempt_at=? WHERE id=?`,attempts,time.Now().UTC().Add(delay).Format(time.RFC3339),id)
 return err
}
// EnqueueTelegram is called inside the monitor transaction, ensuring one
// notification per status transition and no duplicate on repeated failures.
func EnqueueTelegram(ctx context.Context, tx *sql.Tx, kind, msg string) error {
 column:="notify_down"
 if kind=="UP" {column="notify_up"}
 query:=`INSERT INTO telegram_outbox(message,next_attempt_at) SELECT ?,? FROM telegram_settings WHERE id=1 AND enabled=1 AND `+column+`=1`
 _,err:=tx.ExecContext(ctx,query,msg,time.Now().UTC().Format(time.RFC3339))
 return err
}
