---
title: "Architecture"
weight: 10
---

# Architecture

EdS is layered, and the layering is load-bearing rather than decorative. Each
layer is deliberately ignorant of the one below it, which is what lets the
palette extractor be tuned against a folder of JPEGs with no infrastructure at
all, and what lets the stand keep doing something sensible when the network
disappears.
