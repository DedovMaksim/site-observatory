package pageauditwatch

import (
 "context"
 "log"
 "time"

 "example.com/site-observatory/internal/pageaudit"
 "example.com/site-observatory/internal/sitemap"
 "example.com/site-observatory/internal/storage"
)
type Runner struct {DB *storage.DB}
func New(db *storage.DB)*Runner{return &Runner{DB:db}}
func(r *Runner) Run(ctx context.Context){
 r.tick(ctx)
 ticker:=time.NewTicker(time.Minute);defer ticker.Stop()
 for {select{case <-ctx.Done():return;case <-ticker.C:r.tick(ctx)}}
}
func(r *Runner) tick(ctx context.Context){
 pages,err:=r.DB.PageAuditDue(time.Now(),5)
 if err!=nil{log.Printf("page audit queue: %v",err);return}
 for i,item:=range pages{
  if ctx.Err()!=nil{return}
  if i>0{
   t:=time.NewTimer(2*time.Second)
   select{case <-ctx.Done():t.Stop();return;case <-t.C:}
  }
  pageCtx,cancel:=context.WithTimeout(ctx,15*time.Second)
  code,body,headers,e:=sitemap.FetchPage(pageCtx,item.SiteURL,item.URL)
  cancel()
  if ctx.Err()!=nil{return}
  result:=storage.PageAuditResult{URL:item.URL,Status:code}
  if e!=nil {
   result.Error=e.Error()
   if len(result.Error)>400{result.Error=result.Error[:400]}
  }else{
   meta:=pageaudit.ParseSEO(body,headers)
   result.Title=meta.Title
   result.Canonical=meta.Canonical
   result.Robots=meta.Robots
   result.Noindex=pageaudit.Noindex(meta.Robots)
  }
  if err=r.DB.SavePageAudit(ctx,item.MonitorID,result);err!=nil{log.Printf("page audit save: %v",err)}
 }
}
