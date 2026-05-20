#!/bin/sh
#set -x
set -e
find Takeout \
     -name "*.jpg" -print -o \
     -name "*.jpeg" -print -o \
     -name "*.JPG" -print -o \
     -name "*.JPEG" -print -o \
     -name "*.png" -print -o \
     -name "*.PNG" -print -o \
     -name "*.gif" -print -o \
     -name "*.HEIC" -print -o \
     -name "*.heic" -print > /tmp/p.txt
NP=`cat /tmp/p.txt | wc -l`
echo processing $NP photos
rm -f photo-index.txt
touch photo-index.txt
#
I=0
while [ $I -le $NP ];
do
    R=`expr $NP - $I`
    I=`expr $I + 1`
    P=`cat /tmp/p.txt | tail -$R | head -1`
    SHA1=`cat "$P" | sha1`
    echo $SHA1 "$P" >> photo-index.txt
done
