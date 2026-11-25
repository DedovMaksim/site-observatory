package sitemapwatch

import (
 "context"
 "log"
 "time"
 "example.com/site-observatory/internal/sitemap"
 "example.com/site-observatory/internal/storage"
)
type Runner struct{ DB *storage.DB }
func New(db *storage.DB)*Runner{return &Runner{DB:db}}
func (r *Runner) Run(ctx context.Context){
 r.tick(ctx)
 timer:=time.NewTicker(time.Minute)
 defer timer.Stop()
 for{select{case <-ctx.Done():return;case <-timer.C:r.tick(ctx)}}
}
func (r *Runner) tick(ctx context.Context){
 list,err:=r.DB.DueSitemaps(time.Now(),3)
 if err!=nil{log.Printf("sitemap list: %v",err);return}
 for _,site:=range list{
  if ctx.Err()!=nil{return}
  scanCtx,cancel:=context.WithTimeout(ctx,3*time.Minute)
  result,scanErr:=(sitemap.Scanner{}).Scan(scanCtx,site.SiteURL,site.URL)
  cancel()
  if ctx.Err()!=nil{return}
  if scanErr!=nil{
   if err=r.DB.RecordSitemapError(ctx,site.MonitorID,scanErr);err!=nil{log.Printf("sitemap error save: %v",err)}
   continue
  }
  if err=r.DB.RecordSitemap(ctx,site.MonitorID,result.URLs,result.Files);err!=nil{log.Printf("sitemap snapshot save: %v",err)}
 }
}
