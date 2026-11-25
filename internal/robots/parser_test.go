package robots

import (
 "reflect"
 "testing"
)
func TestGroupScoping(t *testing.T){
 body:="User-agent: Googlebot\nUser-agent: Yandex\nDisallow: /\n\nUser-agent: SomeBot\nDisallow: /\n\nUser-agent: *\nDisallow: /private\nSitemap: https://example.com/sitemap.xml"
 got:=Parse(body)
 if !reflect.DeepEqual(got.RootBlocks,[]string{"googlebot","yandex"}){t.Fatalf("root blocks %+v",got.RootBlocks)}
 if len(got.SitemapURLs)!=1{t.Fatalf("sitemap %+v",got.SitemapURLs)}
}
func TestCommentAndBlankDisallow(t *testing.T){
 before:=Parse("User-agent: *\nDisallow:\nDisallow: /admin\n")
 after:=Parse("User-agent: *\nDisallow: / # scary\n")
 if len(before.RootBlocks)!=0{t.Fatal("blank disallow is not a block")}
 if got:=AddedRootBlocks(before,after);!reflect.DeepEqual(got,[]string{"*"}){t.Fatalf("new root %+v",got)}
 if got:=AddedRootBlocks(after,after);len(got)!=0{t.Fatalf("duplicate root %+v",got)}
}
func TestSitemapChangesIgnoresOrder(t *testing.T){
 a:=Parse("Sitemap: https://example.com/1.xml\nSitemap: https://example.com/2.xml")
 b:=Parse("Sitemap: https://example.com/2.xml\nSitemap: https://example.com/1.xml")
 if SitemapChanged(a,b){t.Fatal("order is immaterial")}
}
