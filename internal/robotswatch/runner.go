package robotswatch

import (
 "context"
 "log"
 "time"

 "example.com/site-observatory/internal/sitemap"
 "example.com/site-observatory/internal/storage"
)
type Runner struct { DB *storage.DB }
func New(db *storage.DB)*Runner{return &Runner{DB:db}}
func (r *Runner) Run(ctx context.Context){
 r.tick(ctx)
 ticker:=time.NewTicker(time.Minute)
 defer ticker.Stop()
 for{select{case <-ctx.Done():return;case <-ticker.C:r.tick(ctx)}}
}
func (r *Runner) tick(ctx context.Context){
 due,err:=r.DB.DueRobots(time.Now(),3)
 if err!=nil{log.Printf("robots due: %v",err);return}
 for _,s:=range due{
  if ctx.Err()!=nil{return}
  scanCtx,cancel:=context.WithTimeout(ctx,15*time.Second)
  body,scanErr:=sitemap.FetchRobots(scanCtx,s.SiteURL)
  cancel()
  if ctx.Err()!=nil{return}
  if scanErr!=nil{
   if err=r.DB.RecordRobotsError(ctx,s.MonitorID,scanErr);err!=nil{log.Printf("robots failure save: %v",err)}
  }else if err=r.DB.RecordRobots(ctx,s.MonitorID,body);err!=nil{
   log.Printf("robots snapshot save: %v",err)
  }
 }
}
