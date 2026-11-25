package robots

import (
 "bufio"
 "crypto/sha256"
 "encoding/hex"
 "sort"
 "strings"
)
// Summary deliberately avoids a full RFC 9309 allow/disallow evaluator.
// It reports dangerous literal root Disallow directives in relevant groups.
type Summary struct{
 Hash string
 RootBlocks []string
 SitemapURLs []string
 Disallows []string
}
func Parse(source string) Summary {
 normalized:=strings.ReplaceAll(strings.ReplaceAll(source,"\r\n","\n"),"\r","\n")
 hash:=sha256.Sum256([]byte(normalized))
 result:=Summary{Hash:hex.EncodeToString(hash[:])}
 // Consecutive User-agent directives form one group until first rule.
 agents:=[]string{}
 hasRules:=false
 blockSet:=map[string]bool{}
 disallowSet:=map[string]bool{}
 sitemapSet:=map[string]bool{}
 scanner:=bufio.NewScanner(strings.NewReader(normalized))
 scanner.Buffer(make([]byte,4096),1024*1024)
 for scanner.Scan(){
  line:=strings.TrimSpace(strings.SplitN(scanner.Text(),"#",2)[0])
  if line==""{continue}
  parts:=strings.SplitN(line,":",2)
  if len(parts)!=2{continue}
  key:=strings.ToLower(strings.TrimSpace(parts[0]))
  value:=strings.TrimSpace(parts[1])
  if key=="sitemap"{
   if value!=""{sitemapSet[value]=true}
   continue
  }
  if key=="user-agent"{
   if hasRules {agents=nil;hasRules=false}
   if value!="" {agents=append(agents,strings.ToLower(value))}
   continue
  }
  if key=="allow"||key=="disallow"{
   hasRules=true
   if key!="disallow"||value==""{continue}
   for _,agent:=range agents{
    if agent=="*"||agent=="googlebot"||agent=="yandex" {
     disallowSet[agent+": "+value]=true
     // Exact "/" root disallow is only a strong warning signal,
     // not definitive proof that the site is blocked (Allow may override).
     if value=="/"{blockSet[agent]=true}
    }
   }
  }
 }
 for x:=range blockSet{result.RootBlocks=append(result.RootBlocks,x)}
 for x:=range disallowSet{result.Disallows=append(result.Disallows,x)}
 for x:=range sitemapSet{result.SitemapURLs=append(result.SitemapURLs,x)}
 sort.Strings(result.RootBlocks)
 sort.Strings(result.Disallows)
 sort.Strings(result.SitemapURLs)
 return result
}
func AddedRootBlocks(before,after Summary)[]string{
 known:=map[string]bool{}
 for _,x:=range before.RootBlocks{known[x]=true}
 result:=[]string{}
 for _,x:=range after.RootBlocks{if !known[x]{result=append(result,x)}}
 return result
}
func SitemapChanged(before,after Summary)bool {
 return strings.Join(before.SitemapURLs,"\n")!=strings.Join(after.SitemapURLs,"\n")
}
