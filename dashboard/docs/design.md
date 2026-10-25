# NetBirdy dashboard design system

## Palettes

Dark theme (default): canvas #000000, card #0c0c0c to #1a1a1a, hairlines #1a1a1a to #3a3a3a, text #e8e8e8, secondary #7d7d7d, accent #ff6b7a on dark fills and #d92a41 for fills with white text.

Light theme: canvas #f6f2ea, card #faf7f0, borders #ddd5c3, ink #1d2320, secondary #5a6158, accent #b81e34 with #fbf8f1 text on fills.

Crimson accent ramp (--color-netbirdy-*): 50 #fff1f2, 100 #ffe1e4, 200 #ffc9cf, 300 #ff9ea8, 400 #ff6b7a, 500 #f23c52, 600 #d92a41, 700 #b81e34, 800 #911a2e, 900 #741a2b, 950 #3f0a14. Dark fills use the 500-600 end, accent text on dark uses 400; light fills use 700, accent text on paper uses 700-800.

Info uses the slate blue ramp (--color-nb-blue-*), success stays green, warnings stay amber.

## Semantic color rules

Danger is red and never relies on color alone: destructive actions use the danger-outline button variant (outlined, red text and border, explicit label) and error messages carry an icon. The crimson accent is never used on danger components and red is never used as an accent. Status: connected green, connecting and needs-approval amber, offline and disabled neutral gray, error red with an icon.

## Typography

UI and headings: Instrument Sans variable (vendored woff2). Code and numeric: JetBrains Mono variable (vendored woff2) with tabular-nums in tables. Large headings use tight tracking.

## Patterns

Mesh backdrop: faint static network-tile drawing, radial mask fade toward the center, on auth and full-screen states. Full-row active wash: the active nav item (top-level and nested) gets a rounded highlight of a soft horizontal accent gradient (16% to 4% alpha dark, 11% to 3% light) plus a 1px inset accent hairline at 24% alpha, a medium-weight full-contrast label, and an accent icon. Nested items still sit on a plain 1px hairline rail. Hover on inactive items is a neutral 5% wash. The 1px inset is reserved via inset box-shadow so there is no layout shift. Reticle corners: selected cards show four L-shaped accent corner marks plus a 35 percent accent border. Tabs: the active tab carries a 16 by 2 px accent tick centered under the label. Motion: color and opacity transitions up to 150 ms.

## Contrast (WCAG 2.x)

| Pair | Dark | Light |
|---|---|---|
| body text on canvas | 17.14 | 14.32 |
| body text on card | 15.97 | 14.94 |
| muted text on card | 4.75 | 5.97 |
| accent text on canvas | 7.64 | 5.75 |
| accent text on card | 7.11 | 6.00 |
| primary button text on fill | 4.82 | 6.05 |
| danger text on canvas | 7.59 | 5.79 |
| success text on canvas | 12.05 | 4.49 |
| warning text on canvas | 12.58 | 4.50 |
| info text on canvas | 5.07 | 6.62 |
| hairline on canvas (non-text) | 1.21 | 1.31 |

Text pairs meet 4.5:1 except light-theme success text at 4.49:1 (use green-700 #15803d or darker for small text).
