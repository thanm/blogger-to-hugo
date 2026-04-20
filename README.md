
# blogger-to-hugo

A conversion tool for migrating a Blogger blog to markdown and into Hugo. Designed to be used on the `feed.atom` tool generated from a Blogger takeout.

** NB: still very much under development and not ready for general use (this **
** not to be removed when I feel that the tool is fully baked). ** 

It is worth noting that markdown is a good deal less rich and expressive than HTML; while some HTML constructs map one to one with markdown constructs (ex: bold text) there are other HTML things that don't have a straightforward markdown equivalent (ex: centering an image on the page).  Although in most cases consumers of markdown allow you to add raw HTML, Hugo forbids this by default.

So as to keep things in the spirit of Hugo, when this converted encounters a common bit of HTML that isn't easily translatable, it emits an HTML shortcode to handle it. For example, if it sees

```
<span style="font-family: Arial; font-size: 11pt>
...
</span>
```

It will generate a new shortcode that can be used to surround the content in the span. Resulting markdown:

```
{{< span-arial >}}
...
{{< /span-arial >}}
```
