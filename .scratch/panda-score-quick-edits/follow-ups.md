# Follow-ups

- Run the spec's manual checks 'One line per entry' and 'Columns' in the real app at 1280, 700 and 390 px, in both Chromium and Firefox. Confirm the 12rem floor of the weights table's label cell holds; if it does not, move the min-width onto the inner .score-label span (findings 2, 13, 22).
- Optional: keep the Weights filter state at page level so the filter text also survives a favorites switch that passes through a no-model preview (finding 7).
