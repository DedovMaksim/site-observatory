package diagnostics

import (
 "context"
 "log"
 "sync"
 "time"

 "example.com/site-observatory/internal/storage"
)

type Runner struct {
 DB *storage.DB
 limit chan struct{}
 wait sync.WaitGroup
 last map[int64]time.Time
}
func NewRunner(db *storage.DB)*Runner{return &Runner{DB:db,limit:make(chan struct{},4),last:make(map[int64]time.Time)}}
func (r *Runner) Run(ctx context.Context) {
 r.scan(ctx)
 ticker:=time.NewTicker(time.Minute)
 defer ticker.Stop()
 for {
  select {case <-ctx.Done():r.wait.Wait();return
  case <-ticker.C:r.scan(ctx)}
 }
}
func (r *Runner) scan(ctx context.Context){
 monitors,err:=r.DB.Monitors()
 if err!=nil {log.Printf("diagnostics list: %v",err);return}
 for _,m:=range monitors {
  if !m.Enabled {continue}
  if last:=r.last[m.ID]; !last.IsZero() && time.Since(last)<time.Hour {continue}
  select {case <-ctx.Done():return;case r.limit<-struct{}{}:
   r.last[m.ID]=time.Now()
   r.wait.Add(1)
   go func(m storage.Monitor){
    defer r.wait.Done()
    defer func(){<-r.limit}()
    result:=Run(ctx,m.URL,m.Timeout)
    if ctx.Err()!=nil{return}
    err:=r.DB.SaveDiagnostic(ctx,storage.Diagnostic{MonitorID:m.ID,CheckedAt:result.CheckedAt,TLSStatus:result.TLSStatus,TLSDays:result.TLSDays,TLSExpiry:result.TLSExpiry,DNSSummary:result.DNSSummary,DNSChanged:result.DNSChanged,Title:result.Title,Canonical:result.Canonical,Robots:result.Robots,SEOStatus:result.SEOStatus,Error:result.Error})
    if err!=nil{log.Printf("diagnostic save %d: %v",m.ID,err)}
   }(m)
  }
 }
}
