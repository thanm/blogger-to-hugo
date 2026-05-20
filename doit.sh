#!/bin/sh
set -x
set -e
rm -rf out
go build
./blogger-to-hugo -v=1 -infile=feed.atom -outdir=out -wrphotos=photo-urls.txt -entlim=10 1> e.txt 2>&1
cp out/*.md tblog/content/posts
cp out/shortcodes/* tblog/layouts/shortcodes
cd tblog
hugo server
