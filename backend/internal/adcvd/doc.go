// Package adcvd flags parts that may be subject to a U.S. antidumping (AD) or
// countervailing duty (CVD) order, from the part's HTS code and its country
// of origin. It is a screen, not a finding: it matches on the HTS numbers a
// Commerce order notice prints in the scope of an order, which are given for
// convenience and customs purposes only; the written scope of each order
// governs, and only Commerce decides whether a product is within it.
//
// The table in data/orders.json is built from public Federal Register
// notices by eval/adcvd/build_orders.py: Commerce's monthly lists of orders
// open to administrative review, its order and revocation notices, and the
// newest notice for each order that carries the scope text. The file records
// the snapshot date, each order's source notice and the verbatim scope
// paragraphs the HTS numbers were read from.
//
// Display only (D939): nothing here changes the origin answer or a duty
// figure. A part is possible exposure when an order's listed HTS number
// agrees with the part's code at the shorter of the two lengths and the
// order's country is one of the part's countries of origin. No model call.
package adcvd
