package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/beevik/etree"
)

var infileflag = flag.String("infile", "", "Input XML file")
var outdirflag = flag.String("outdir", "", "Output directory for posts")
var verbflag = flag.Int("v", 0, "Verbose trace output level")
var entlimitflag = flag.Int("entlim", 0, "Stop after processing N entries (debugging)")

type state struct {
	entries  []*etree.Element
	pubdates []time.Time
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

func (s *state) walkxml(root *etree.Document) error {
	for feeds := range root.SelectElementsSeq("feed") {
		for ent := range feeds.SelectElementsSeq("entry") {
			fmt.Println("CHILD element:", ent.Tag)
			if title := ent.SelectElement("title"); title != nil {
				lang := title.SelectAttrValue("lang", "unknown")
				fmt.Printf("  TITLE: %s (%s)\n", title.Text(), lang)
			}
			if id := ent.SelectElement("id"); id != nil {
				fmt.Printf("  ID: %s\n", id.Text())
			}
			s.entries = append(s.entries, ent)
			if *entlimitflag != 0 && len(s.entries) >= *entlimitflag {
				verb(0, "note: stopped appending entries at %d count", *entlimitflag)
				break
			}
			panic("do this stuff below")
			// locate publication date
			// collect blogger:filename and set correct slug (year, month, title fragment)
		}
	}
	return nil
}

func (s *state) emitEntry() error {
	// collect categories
	// note: explore using hugo taxonomies
	// walk content, converting to markdown
	return nil
}

func (s *state) emit() error {
	for _, ent := range s.entries {
		if err := s.emitEntry(ent); err != nil {
			return err
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
