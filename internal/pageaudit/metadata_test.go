package pageaudit

import (
 "net/http"
 "testing"
)
func TestParseSEO(t *testing.T){
 body:=`<!doctype html><html><head><title>Recipe &amp; Spices</title><link href='https://example.com/canonical' rel='canonical'><meta content="index, follow" name="robots"></head></html>`
 got:=ParseSEO(body,http.Header{"X-Robots-Tag":[]string{"noindex, nofollow"}})
 if got.Title!="Recipe & Spices" || got.Canonical!="https://example.com/canonical" || !Noindex(got.Robots){t.Fatalf("metadata %+v",got)}
}
func TestNoindex(t *testing.T){
 if Noindex("index,follow"){t.Fatal("index is not noindex")}
 if !Noindex("googlebot: noindex, follow"){t.Fatal("noindex not detected")}
}
