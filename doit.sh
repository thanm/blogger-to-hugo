#!/bin/sh
set -x
set -e
rm -rf out
go build
./blogger-to-hugo -v=1 -infile=feed.atom -outdir=out -wrphotos=photo-urls.txt 1> e.txt 2>&1
mkdir -p tblog/content/posts
cp out/*.md tblog/content/posts
mkdir -p tblog/layouts/shortcodes
cp out/shortcodes/* tblog/layouts/shortcodes
mkdir -p tblog/themes
if [ ! -d tblog/themes/hugo-PaperMod ]; then
    pushd tblog/themes
    git clone https://github.com/adityatelange/hugo-PaperMod
    popd
fi
cd tblog
hugo server
