# Website language detection

## Completed

The website's English route (`website/index.html`) now selects the Simplified Chinese route (`website/zh-CN/`) when the browser's preferred language list contains a Chinese locale; it otherwise keeps the English route. This lets the browser carry through the visitor's OS/browser language preference on the first visit.

The language toggle persists an explicit choice in `localStorage`. A saved manual choice always takes precedence over automatic detection on later visits, including when visitors open a localized URL directly.

## Problems encountered

None.

## Blockers or unfinished work

None.
