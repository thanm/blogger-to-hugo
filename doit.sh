#!/bin/sh
set -x
set -e
rm -rf out
go build
./blogger-to-hugo -v=1 -infile=feed.atom -outdir=out -entlim=10 1> e.txt 2>&1
cp out/* tblog/content/posts
cd tblog
hugo server
