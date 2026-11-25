package pageaudit

import (
 "html"
 "net/http"
 "regexp"
 "strings"
)
type SEO struct {
 Title,Canonical,Robots string
}
var titlePattern=regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
var tagPattern=regexp.MustCompile(`(?is)<(?:meta|link)\b[^>]*>`)
var attrPattern=regexp.MustCompile(`(?is)([\w:-]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var spacePattern=regexp.MustCompile(`\s+`)
func attrs(tag string)map[string]string{
 out:=map[string]string{}
 for _,m:=range attrPattern.FindAllStringSubmatch(tag,-1){
  val:=m[2]
  if val==""{val=m[3]}
  if val==""{val=m[4]}
  key:=strings.ToLower(m[1])
  if _,ok:=out[key];!ok{out[key]=html.UnescapeString(strings.TrimSpace(val))}
 }
 return out
}
func ParseSEO(body string,header http.Header)SEO{
 s:=SEO{}
 // Lightweight metadata extraction; does not execute JavaScript.
 // It is advisory and can miss dynamically rendered metadata.
 if m:=titlePattern.FindStringSubmatch(body);len(m)>1{
  s.Title=spacePattern.ReplaceAllString(strings.TrimSpace(html.UnescapeString(m[1]))," ")
  if len(s.Title)>400{s.Title=s.Title[:400]}
 }
 for _,tag:=range tagPattern.FindAllString(body){
  a:=attrs(tag)
  low:=strings.ToLower(tag)
  if strings.HasPrefix(low,"<link") && s.Canonical==""{
   for _,token:=range strings.Fields(strings.ToLower(a["rel"])){
    if token=="canonical"{s.Canonical=a["href"];break}
   }
  }
  if strings.HasPrefix(low,"<meta"){
   name:=strings.ToLower(a["name"])
   if name=="robots"||name=="googlebot"||name=="yandex"{
    if a["content"]!=""{
     if s.Robots!=""{s.Robots+="; "}
     s.Robots+=name+": "+a["content"]
    }
   }
  }
 }
 if x:=header.Values("X-Robots-Tag");len(x)>0{
  if s.Robots!=""{s.Robots+="; "}
  s.Robots+="X-Robots-Tag: "+strings.Join(x,", ")
 }
 if len(s.Canonical)>2048{s.Canonical=s.Canonical[:2048]}
 if len(s.Robots)>1000{s.Robots=s.Robots[:1000]}
 return s
}
func Noindex(robots string)bool{
 return regexp.MustCompile(`(?i)(^|[^a-z])noindex([^a-z]|$)`).MatchString(robots)
}
