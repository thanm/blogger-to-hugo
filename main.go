package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Todo list
// - create output filename based on slug/year/day, write text to it
// - front matter for markdown post (title etc)
// - support for tags, organize by year and month similar to blogger
//

///
// Thoughts on anchor handling:
// - anchors are used for regular links and for image links, these need
//   to be handled differently.
// - when walking the tree for an anchor we'll see the "A" node first in pre,
//   with its attributes, then visit the text in the anchor (pre and post),
//   then visit the "A" in post
//   + for the text in the anchor data we want to not emit it right away,
//     but instead buffer for later processing
//
// Thoughts on image handling:
//
// HTML generated for images looks something like this:
//
//  <div class="separator" style="clear: both; text-align: center;">
//  <a href="URL" imageanchor="1" style="margin-left: 1em; margin-right: 1em;">
//  <img border="0" data-original-height="1200" data-original-width="1600" height="300" src="URL" width="400">
//  </a><
//  /div>          /* NBL: imageanchor=1 is a non-standard attr */
//
// where URL is a blogger image such as
//
//  https://blogger.googleusercontent.com/img/b/R29vZ2xl/AVvXsEiQcGNs9bHzi2eq1eyM2wB4UNIhMlIaTlrr7lENOhUlQozHtuoKqyofWQHHkqrh30fuWWeh49KlPMqqxfohnegw5OsWLUg4uHL2pwuvcA4XjED5_Hvji1IIQTUTSCxWttYZ1_HCTB9IdGs/s400/IMG_20171123_065124.jpg
//
// where "s400" or "s1600" indicates resolution.
//
// - image file itself appears somewhere in the Takeout directory, however we
//   can't locate strictly by name since there may be collisions
//   or duplicates (ex: 214.jpg, which appears in a couple of distinct URLs)
// - it should be possible to write a disambiguation phase, e.g. collect up
//   potential duplicates in a pre-pass and then when we hit a dup, do a call to
//   html.Get() to collect the bytes and then build an appropriate mapping
// - would need to figure out some sort of strategy for picking new photo
//   names so that the final namespace doesn't have any duplicates
//
// As a first step I could just keep the googleusercontent.com URL and
// emit a shortcode that handles all the other stuff (alignment, width, etc).
// shortcode parameters/inputs:
//   - image URL
//   - anchor style
//   - image src
//   - img border attr
//   - img data-original-height, data-original-width
//   - image width attr
//   - image height attr
// Q: what happens if not all attrs present?
//
// Thoughts about hosting: seems as though putting all the images up on flickr
// might be the best way do go, although that requires a yearly cost of around
// 80 bucks.
//
//  - note: is there a way I could arrange for a level of indirection here,
//    e.g. link in blog is to https://wanderingsquid.org/photofwd/<HASH>
//    which then is instantly redirected to either googleusercontent.com or
//    flickr.com or whatever depending? Doesn't look as though this is all
//    that easy.
//  - alternatively I could write a tool that rewrites URL paths from
//    one service to another within the markdown source; this could then
//    be done if/when I need to move photos.
//
// Revamped thoughts about images and the Takeout directory:
// - question: is Takeout version larger or smaller?
// - file in Takeout are different from the ones being actually served by Blogger;
//   this means that there isn't any sort of trivial or "slam dunk" way to
//   tell whether Blogger's "214.jpg" is the same photo as Takeout's "214.jpg", since
//   the two will be at a different resolution with different meta-data.
//

var infileflag = flag.String("infile", "", "Input XML file")
var outdirflag = flag.String("outdir", "", "Output directory for posts")
var wrphotosflag = flag.String("wrphotos", "", "Write photos URL output file")
var verbflag = flag.Int("v", 0, "Verbose trace output level")
var entlimitflag = flag.Int("entlim", 0, "Stop after processing N entries (debugging)")

const usebloggerphotos = true

// Shortcodes for pointing to a blogger copy of a photo
const blphshortcode = "bloggerphoto"

// Photo shortcode for flickr (not yet implemented)
const flshortcode = "flickrphoto"

const startdivshortcode = "startdiv"
const enddivshortcode = "enddiv"
const clearcenterdivshortcode = "clearcenterdiv"
const clearleftdivshortcode = "clearleftdiv"
const clearstartdivshortcode = "clearstartdiv"
const clearnonedivshortcode = "clearnonediv"

var divstyles = map[string]string{
	"clear: both; text-align: center;": "clearcenterdiv",
	"clear: both; text-align: start;":  "clearstartdiv",
	"clear: both; text-align: left;":   "clearleftdiv",
	"clear: both;":                     "clearnonediv",
}

type bentry struct {
	year, month int
	urlfrag     string
	title       string
	pubdate     time.Time
	tags        []string
	elem        *etree.Element
}

type state struct {
	bentries []bentry
}

func readxml(path string) (*etree.Document, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromFile(path); err != nil {
		return nil, err
	}
	return doc, nil
}

type at struct {
	key, val string
}

type imgdata struct {
	ats []at
	src string
}

type anchordata struct {
	href  string
	md    strings.Builder
	img   imgdata
	style string
}

type hvisitor struct {
	spanstack []string
	ad        anchordata
	md        strings.Builder
	uatoms    map[atom.Atom]struct{}
	uspanats  map[string]struct{}
	uanchats  map[string]struct{}
	imurls    map[string]struct{}
}

var guatoms map[atom.Atom]struct{}
var guspanats map[string]struct{}
var guanchats map[string]struct{}
var gimurls map[string]struct{}

func mkhvisitor() *hvisitor {
	if guatoms == nil {
		guatoms = make(map[atom.Atom]struct{})
		guspanats = make(map[string]struct{})
		guanchats = make(map[string]struct{})
		gimurls = make(map[string]struct{})
	}
	return &hvisitor{
		uatoms:   guatoms,
		uspanats: guspanats,
		uanchats: guanchats,
		imurls:   gimurls,
	}
}

func (hv *hvisitor) unsupportedAtom(a atom.Atom) {
	if _, ok := hv.uatoms[a]; !ok {
		verb(1, " => unsupported atom ignored: %v", a)
		hv.uatoms[a] = struct{}{}
	}
}

func (hv *hvisitor) unsupportedSpanAttr(key, val string) {
	k := key + " -> " + val
	if _, ok := hv.uspanats[k]; !ok {
		verb(1, " => unsupported span attr ignored: %q", k)
		hv.uspanats[k] = struct{}{}
	}
}

func (hv *hvisitor) unsupportedAnchorAttr(key, val string) {
	k := key + " -> " + val
	if _, ok := hv.uanchats[k]; !ok {
		verb(1, " => unsupported anchor attr ignored: %q", k)
		hv.uanchats[k] = struct{}{}
	}
}

func (hv *hvisitor) emitAnchor() error {
	if hv.ad.href == "" || hv.ad.img.src == "" {
		// not an image -- emit as regular hyperlink.
		fmt.Fprintf(&hv.md, " [%s](%s) ",
			hv.ad.md.String(), hv.ad.href)
		return nil
	}
	return hv.emitImage()
}

func (hv *hvisitor) emitImage() error {
	// pseudocode:
	// + href and img.src must be set
	// + emit shortcode if not already emitted
	hv.imurls[hv.ad.href] = struct{}{}

	// Split up the href URL into components, then pick out the last one.
	parts := strings.Split(hv.ad.href, "/")
	fname := parts[len(parts)-1]

	verb(1, "^ completing anchor: fname=%q url=%q style=%q imgsrc=%q ats=%v",
		fname, hv.ad.href, hv.ad.style, hv.ad.img.src, hv.ad.img.ats)

	if usebloggerphotos {
		// Shortcode parameters:
		// s1600 - url for full sized s1600 photo
		// s400 - url for smaller s400 photo
		// style - style string
		// border - image border
		// height - height
		// width - width
		// data-original-height - data-original-height
		// data-original-width - data-original-width
		getat := func(k string) string {
			for _, a := range hv.ad.img.ats {
				if a.key == k {
					return a.val
				}
			}
			return ""
		}

		// Emit a shortcode for the photo. This version simply points to the
		// original blogger photo.
		fmt.Fprintf(&hv.md, "{{< %s s1600=\"%s\" s400=\"%s\" style=\"%s\" border=\"%s\" height=\"%s\" width=\"%s\" data-original-height=\"%s\" data-original-width=\"%s\"  >}}\n",
			blphshortcode,
			hv.ad.href,
			hv.ad.img.src,
			hv.ad.style,
			getat("border"),
			getat("height"),
			getat("width"),
			getat("data-original-height"),
			getat("data-original-width"))
		fmt.Fprintf(&hv.md, "\n")
	} else {
		// Shortcode to refer to flickr photo.
		panic("not yet implemented")
	}

	// FIXME: issue error in case s400/s1600 not set

	return nil

}

func (hv *hvisitor) pre(n *html.Node, level int) error {
	verb(1, "pre: lev=%d nodetype %s atom: %v", level, n.Type, n.DataAtom)
	switch n.Type {
	case html.TextNode:
		// QQ do we need to worry about escaping here?
		verb(1, "level %d textnode %q", level, n.Data)
		if hv.ad.href != "" {
			hv.ad.md.WriteString(n.Data)
		} else {
			hv.md.WriteString(n.Data)
		}
	case html.ElementNode:
		switch n.DataAtom {
		case atom.A:
			verb(1, "+ begin anchor")
			hv.ad = anchordata{}
			// collect anchor attributes
			for _, a := range n.Attr {
				switch a.Key {
				case "href":
					hv.ad.href = a.Val
				case "style":
					hv.ad.style = a.Val
				case "imageanchor":
					// this is a blogger-specific non-standard attr that shows up for
					// some reason -- we can ignore it.
				default:
					hv.unsupportedAnchorAttr(a.Key, a.Val)
				}
			}
		case atom.B:
			hv.md.WriteString("**")
		case atom.Br:
			hv.md.WriteString("\n")
		case atom.Img:
			for _, a := range n.Attr {
				hv.ad.img.ats = append(hv.ad.img.ats, at{key: a.Key, val: a.Val})
				if a.Key == "src" {
					hv.ad.img.src = a.Val
				}
			}
		case atom.Div:
			// <div class="..." style="..."> stuff </div>
			// ideally we would handle this as two shortcodes 'startdiv' and 'enddiv',
			// but golang templating is very picky about doing style={{param}}, and
			// refuses to allow arbitrary values. Instead pick out specific scenarios
			// common for photos and support those.
			dclass := ""
			dstyle := ""
			for _, a := range n.Attr {
				switch a.Key {
				case "class":
					dclass = a.Val
				case "style":
					dstyle = a.Val
				}
			}
			shortcodename := startdivshortcode
			if v, ok := divstyles[dstyle]; ok {
				shortcodename = v
			}
			fmt.Fprintf(&hv.md, "{{< %s class=%q style=%q >}}\n", shortcodename,
				dclass, dstyle)
		case atom.Span:
			// ex: <span style="font-weight:bold;">foobar</span>
			for _, a := range n.Attr {
				found := false
				if a.Key == "style" && a.Val == "font-weight:bold;" {
					verb(1, "^^ start bold")
					hv.md.WriteString("**")
					found = true
				}
				if !found {
					hv.unsupportedSpanAttr(a.Key, a.Val)
				}
			}
		default:
			hv.unsupportedAtom(n.DataAtom)
		}
	}
	return nil
}

func (hv *hvisitor) post(n *html.Node, level int) error {
	verb(1, "post: lev=%d nodetype %s atom: %v", level, n.Type, n.DataAtom)
	if n.Type == html.ElementNode {
		switch n.DataAtom {
		case atom.A:
			verb(1, "+ finish anchor")
			if err := hv.emitAnchor(); err != nil {
				return err
			}
			hv.ad = anchordata{}
		case atom.B:
			hv.md.WriteString("**")
		case atom.Div:
			fmt.Fprintf(&hv.md, "{{< %s >}}\n", enddivshortcode)
		case atom.Span:
			// ex: <span style="font-weight:bold;">foobar</span>
			for _, a := range n.Attr {
				if a.Key == "style" && a.Val == "font-weight:bold;" {
					verb(1, "^^ term bold")
					hv.md.WriteString("**")
				}
			}
		}
	}
	return nil
}

func (hv *hvisitor) visitHtmlNode(n *html.Node, level int) {
	hv.pre(n, level)
	if n.FirstChild != nil {
		hv.visitHtmlNode(n.FirstChild, level+1)
	}
	hv.post(n, level)
	if n.NextSibling != nil {
		hv.visitHtmlNode(n.NextSibling, level)
	}
}

func convertPost(ent bentry) (cerr error) {

	// Grab post content.
	var content string
	if celem := ent.elem.SelectElement("content"); celem == nil {
		return fmt.Errorf("entry lacks content element")
	} else {
		content = celem.Text()
	}

	// Grab post title.
	var title string
	if telem := ent.elem.SelectElement("title"); telem == nil {
		return fmt.Errorf("entry lacks title element")
	} else {
		title = telem.Text()
		verb(1, "title is %q", title)
	}

	// Kick off HTML parse of content.
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("html.Parse failed: %v", err)
	}
	verb(1, "converting post: %d/%d %s", ent.year, ent.month, ent.title)

	// Generate head matter.
	v := mkhvisitor()
	fmt.Fprintf(&v.md, "---\n")
	fmt.Fprintf(&v.md, "title: %s\n", strconv.Quote(title))
	fmt.Fprintf(&v.md, "date: %v\n", ent.pubdate)
	if len(ent.tags) > 0 {
		fmt.Fprintf(&v.md, "tags: [")
		idx := 0
		for _, tagval := range ent.tags {
			if idx != 0 {
				fmt.Fprintf(&v.md, " ,")
			}
			idx++
			fmt.Fprintf(&v.md, "%s", strconv.Quote(tagval))
		}
		fmt.Fprintf(&v.md, "]\n")
	}
	fmt.Fprintf(&v.md, "---\n\n")

	// Walk tree.
	v.visitHtmlNode(doc, 0)

	// Open output file.
	fn := fmt.Sprintf("%s/post_y%d_m%d.md", *outdirflag, ent.month, ent.year)
	outf, err := os.OpenFile(fn, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("opening %s: %v", fn, err)
	}

	closer := func() {
		if err := outf.Close(); err != nil {
			cerr = fmt.Errorf("closing %s: %v", fn, err)
		}
	}
	defer closer()

	// Write markdown
	if _, err := fmt.Fprintf(outf, "%s\n", v.md.String()); err != nil {
		return fmt.Errorf("writing to %s: %v", fn, err)
	}
	return nil
}

func (s *state) addEntry(elem *etree.Element) error {

	var b bentry
	b.elem = elem

	// locate and parse publication date. we expect a format something like
	// <published>2009-06-01T12:13:00.003Z</published>
	if published := elem.SelectElement("published"); published != nil {
		txt := published.Text()
		verb(1, "entry pubdate: %s", txt)
		t, err := time.Parse(time.RFC3339, txt)
		if err != nil {
			return fmt.Errorf("parsing pubdate: %v", err)
		}
		b.pubdate = t
	} else {
		return fmt.Errorf("entry contains no publication date/time")
	}

	// collect blogger:filename and set correct slug (year, month,
	// title fragment)
	if bfn := elem.SelectElement("blogger:filename"); bfn != nil {
		txt := bfn.Text()
		verb(1, "blogger:filename: %s", txt)
		// expected format: <blogger:filename>/2014/11/braces-off.html</blogger:filename>
		if n, err := fmt.Sscanf(txt, "/%d/%d/%s", &b.month, &b.year, &b.urlfrag); err != nil {
			return fmt.Errorf("unexpected blogger:filename entry %q", txt)
		} else if n != 3 {
			return fmt.Errorf("unexpected partial blogger:filename entry %q", txt)
		}
	} else {
		return fmt.Errorf("entry contains no blogger:filename entry")
	}

	// collect post tags. we're looking for entries of the form
	// <category scheme='tag:blogger.com,<text>' term='swimming'/>
	//if bfn := elem.SelectElement("blogger:filename"); bfn != nil {
	for cat := range elem.SelectElementsSeq("category") {
		if at := cat.SelectAttr("term"); at != nil {
			b.tags = append(b.tags, at.Value)
		}
	}
	if len(b.tags) > 0 {
		verb(1, "tags: %+v", b.tags)
	}

	s.bentries = append(s.bentries, b)

	return nil
}

func (s *state) emitEntry(ent bentry) error {
	// collect categories
	// note: explore using hugo taxonomies
	// walk content, converting to markdown
	if err := convertPost(ent); err != nil {
		return err
	}
	return nil
}

const blphotosshortcodetext = `
{{ $s1600 := .Get "s1600" }}
{{ $s400 := .Get "s400" }}
{{ $style := .Get "style" | default "" }}
{{ $border := .Get "border" | default "" }}
{{ $height := .Get "height" | default "" }}
{{ $width := .Get "width" | default "" }}
{{ $doh := .Get "data-original-height" | default "" }}
 {{ $dow := .Get "data-original-width" | default "" }}

<a href= "{{ $s1600 }}"

 {{ if ne $style "" }}
   style="{{ $style }}" 
 {{ end }}

> <img 

 {{ if ne $border "" }}
   border="{{ $border }}"
 {{ end }}

 src="{{ $s400 }}"

 {{ if ne $height "" }}
   height="{{ $height }}"
 {{ end }}

 {{ if ne $width "" }}
   width="{{ $width }}"
 {{ end }}

 {{ if ne $doh "" }}
   data-original-height="{{ $doh }}"
 {{ end }}

 {{ if ne $dow "" }}
   data-original-width="{{ $dow }}"
 {{ end }}

></a>
`

const startdivshortcodetext = `
{{ $cla := .Get "class" | default "" }}

<div class="{{ $cla | safeHTMLAttr }}">
`

const enddivshortcodetext = `
</div>
`

const clearcenterdivshortcodetext = `
{{ $cla := .Get "class" | default "" }}

<div class="{{ $cla | safeHTMLAttr }}" style="clear: both; text-align: center;">
`

const clearleftdivshortcodetext = `
{{ $cla := .Get "class" | default "" }}

<div class="{{ $cla | safeHTMLAttr }}" style="clear: both; text-align: left;">
`

const clearstartdivshortcodetext = `
{{ $cla := .Get "class" | default "" }}

<div class="{{ $cla | safeHTMLAttr }}" style="clear: both; text-align: start;">
`

const clearnonedivshortcodetext = `
{{ $cla := .Get "class" | default "" }}

<div class="{{ $cla | safeHTMLAttr }}" style="clear: both;">
`

func (s *state) emitShortCode(name, text string) error {
	shcodepath := path.Join(*outdirflag, "shortcodes", name+".html")
	shoutf, err := os.OpenFile(shcodepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("opening shortcodes file %s: %v", shcodepath, err)
	}
	fmt.Fprintf(shoutf, text)

	if err := shoutf.Close(); err != nil {
		return fmt.Errorf("closing %s: %v", shcodepath, err)
	}

	return nil
}

func (s *state) emit() error {
	for _, ent := range s.bentries {
		if err := s.emitEntry(ent); err != nil {
			return err
		}
	}
	if len(gimurls) != 0 && *wrphotosflag != "" {
		outf, err := os.OpenFile(*wrphotosflag, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("opening photos file %s: %v", *wrphotosflag, err)
		}
		urls := make([]string, 0, len(gimurls))
		for k := range gimurls {
			urls = append(urls, k)
		}
		sort.Strings(urls)
		for _, v := range urls {
			fmt.Fprintf(outf, "%s\n", v)
		}
		if err := outf.Close(); err != nil {
			return fmt.Errorf("closing photos file %s: %v", *wrphotosflag, err)
		}
	}
	shd := path.Join(*outdirflag, "shortcodes")
	if shderr := os.Mkdir(shd, 0777); shderr != nil {
		return fmt.Errorf("creating dir %s: %v", shd, shderr)
	}
	toemit := []struct{ n, t string }{
		{blphshortcode, blphotosshortcodetext},
		{startdivshortcode, startdivshortcodetext},
		{enddivshortcode, enddivshortcodetext},
		{clearnonedivshortcode, clearnonedivshortcodetext},
		{clearleftdivshortcode, clearleftdivshortcodetext},
		{clearstartdivshortcode, clearstartdivshortcodetext},
		{clearcenterdivshortcode, clearcenterdivshortcodetext},
	}
	for _, e := range toemit {
		if err := s.emitShortCode(e.n, e.t); err != nil {
			return err
		}
	}
	return nil
}

func (s *state) walkxml(root *etree.Document) error {

	// Collect blog post entries.
	for feeds := range root.SelectElementsSeq("feed") {
		for ent := range feeds.SelectElementsSeq("entry") {
			// skip things that are not posts
			if bt := ent.SelectElement("blogger:type"); bt != nil {
				btt := bt.Text()
				if btt != "POST" {
					verb(1, "ignoring %s entry", btt)
					continue
				}
			}
			// skip drafts for now
			if bs := ent.SelectElement("blogger:status"); bs != nil {
				bst := bs.Text()
				if bst != "LIVE" {
					verb(1, "ignoring blog with status %s", bst)
					continue
				}
			}
			if err := s.addEntry(ent); err != nil {
				return err
			}
			if *entlimitflag != 0 && len(s.bentries) >= *entlimitflag {
				verb(0, "note: stopped appending entries at %d count", *entlimitflag)
				break
			}
		}
	}

	// Sort entries based on publication date.
	sort.Slice(s.bentries, func(i, j int) bool {
		bi := &s.bentries[i]
		bj := &s.bentries[j]
		if bi.pubdate != bj.pubdate {
			return bj.pubdate.Compare(bi.pubdate) < 0
		}
		return bi.title < bj.title
	})

	if *verbflag != 0 {
		verb(1, "entries:")
		for i := range s.bentries {
			b := s.bentries[i]
			verb(1, "%d: %d/%d pd=%v %q", i, b.year, b.month, b.pubdate, b.title)
		}
	}

	return nil
}

func usage(msg string) {
	if len(msg) > 0 {
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	}
	fmt.Fprintf(os.Stderr, "usage: blogger-to-hugo -infile <file> [flags]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func verb(vlevel int, s string, a ...any) {
	if *verbflag >= vlevel {
		fmt.Printf(s, a...)
		fmt.Printf("\n")
	}
}

func main() {
	flag.Parse()
	if *infileflag == "" {
		usage("supply input argument with -infile flag")
	}
	if *outdirflag == "" {
		usage("supply output directory argument with -outdir flag")
	}
	verb(1, "reading %s", *infileflag)
	doc, err := readxml(*infileflag)
	if err != nil {
		log.Fatalf("error reading XML input file %s: %s\n",
			*infileflag, err)
	}
	verb(1, "walking %s", *infileflag)
	s := &state{}
	if err := s.walkxml(doc); err != nil {
		log.Fatalf("error walking XML input file %s: %s\n",
			*infileflag, err)
	}
	verb(1, "writing to outdir %s", *outdirflag)
	if err := os.Mkdir(*outdirflag, 0777); err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}
	if err := s.emit(); err != nil {
		log.Fatalf("error during emit phase: %s\n", err)
	}
	verb(1, "done")
}
