package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/beevik/etree"
)

var infileflag = flag.String("infile", "", "Input XML file")
var outdirflag = flag.String("outdir", "", "Output directory for posts")
var verbflag = flag.Int("v", 0, "Verbose trace output level")
var entlimitflag = flag.Int("entlim", 0, "Stop after processing N entries (debugging)")

type bentry struct {
	year, month int
	title       string
	pubdate     time.Time
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

func convertpost(entry *etree.Element) error {
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

	// collect blogger:filename and set correct slug (year, month, title fragment)
	if bfn := elem.SelectElement("blogger:filename"); bfn != nil {
		txt := bfn.Text()
		verb(1, "blogger:filename: %s", txt)
		// expected format: <blogger:filename>/2014/11/braces-off.html</blogger:filename>
		if n, err := fmt.Sscanf(txt, "/%d/%d/%s", &b.year, &b.month, &b.title); err != nil {
			return fmt.Errorf("unexpected blogger:filename entry %q", txt)
		} else if n != 3 {
			return fmt.Errorf("unexpected partial blogger:filename entry %q", txt)
		}
	} else {
		return fmt.Errorf("entry contains no blogger:filename entry")
	}

	s.bentries = append(s.bentries, b)

	return nil
}

func (s *state) emitEntry(ent bentry) error {
	// collect categories
	// note: explore using hugo taxonomies
	// walk content, converting to markdown
	return nil
}

func (s *state) emit() error {
	for _, ent := range s.bentries {
		if err := s.emitEntry(ent); err != nil {
			return err
		}
	}
	return nil
}

func (s *state) walkxml(root *etree.Document) error {

	// Collect blog post entries.
	for feeds := range root.SelectElementsSeq("feed") {
		for ent := range feeds.SelectElementsSeq("entry") {
			if bt := ent.SelectElement("blogger:type"); bt != nil {
				btt := bt.Text()
				if btt != "POST" {
					verb(1, "ignoring %s entry", btt)
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
			return bi.pubdate.Compare(bj.pubdate) < 0
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
	s.emit()
	if err := s.emit(); err != nil {
		log.Fatalf("error during emit phase: %s\n", err)
	}
	verb(1, "done")
}
