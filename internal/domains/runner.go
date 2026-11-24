package domains

import (
 "context"
 "log"
 "time"

 "example.com/site-observatory/internal/storage"
)

type Runner struct {
 DB *storage.DB
 Lookup *Client
}
func NewRunner(db *storage.DB)*Runner{return &Runner{DB:db,Lookup:NewClient()}}
func (r *Runner) Run(ctx context.Context){
 r.step(ctx)
 tick:=time.NewTicker(time.Minute)
 defer tick.Stop()
 for {
  select {
  case <-ctx.Done():return
  case <-tick.C:r.step(ctx)
  }
 }
}
func (r *Runner) step(ctx context.Context){
 due,err:=r.DB.DueDomains(time.Now(),10)
 if err!=nil{log.Printf("domain expiry lookup list: %v",err);return}
 for _,item:=range due {
  if ctx.Err()!=nil{return}
  lookupCtx,cancel:=context.WithTimeout(ctx,25*time.Second)
  result,lookupErr:=r.Lookup.Lookup(lookupCtx,item.Domain)
  cancel()
  if err:=r.DB.RecordDomainLookup(ctx,item.Domain,result.Expires,result.Source,lookupErr);err!=nil{
   log.Printf("domain expiry save %s: %v",item.Domain,err)
  }
 }
 if err:=r.DB.QueueDomainAlerts(ctx,time.Now());err!=nil{
  log.Printf("domain expiry notification scheduling: %v",err)
 }
}
