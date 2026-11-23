package usmca

// Synthetic fixtures only. Chapter 77 is reserved/unused in the HS, so none of
// these rows resemble real pilot rules or thresholds.
const syntheticRules = `{
 "schema_version": 1,
 "data_version": "synthetic-test-1",
 "snapshot": {"source": "synthetic", "revision": "t1", "retrieved_on": "2026-01-01", "source_hash": "00"},
 "rows": [
  {"id":"S-CH","code":"7701","description":"chapter shift","source":{"citation":"synthetic","excerpt":"A change from any other chapter."},
   "alternatives":[{"id":"a1","label":"chapter","conditions":[{"kind":"change_from_chapter"}]}]},
  {"id":"S-HD","code":"7702","description":"heading shift","source":{"citation":"synthetic","excerpt":"A change from any other heading."},
   "alternatives":[{"id":"a1","label":"heading","conditions":[{"kind":"change_from_heading"}]}]},
  {"id":"S-SH","code":"770331","description":"subheading shift","source":{"citation":"synthetic","excerpt":"A change from any other subheading."},
   "alternatives":[{"id":"a1","label":"subheading","conditions":[{"kind":"change_from_subheading"}]}]},
  {"id":"S-TI","code":"77041010","description":"tariff item shift","source":{"citation":"synthetic","excerpt":"A change from any other tariff item."},
   "alternatives":[{"id":"a1","label":"tariff item","conditions":[{"kind":"change_from_tariff_item"}]}]},
  {"id":"S-SIB","code":"770531","description":"sibling","source":{"citation":"synthetic","excerpt":"sibling rule"},
   "alternatives":[
    {"id":"a1","label":"sibling parts","conditions":[{"kind":"sibling_parts","at_least_one_from":["770539"],"others_from":["770590"],"others_outside_chapter":"77"}]},
    {"id":"a2","label":"rvc no shift","conditions":[{"kind":"no_shift_required"},{"kind":"forbidden_inputs","same_as_good_level":"heading"},{"kind":"rvc","thresholds":{"transaction_value":"60","net_cost":"50"}}]}
   ]},
  {"id":"S-RVC","code":"7706","description":"shift or rvc","source":{"citation":"synthetic","excerpt":"x"},
   "alternatives":[
    {"id":"a1","label":"heading shift","conditions":[{"kind":"change_from_heading"}]},
    {"id":"a2","label":"rvc","conditions":[{"kind":"no_shift_required"},{"kind":"rvc","thresholds":{"transaction_value":"60","net_cost":"50"}}]}
   ]},
  {"id":"S-TVONLY","code":"7707","description":"tv only","source":{"citation":"synthetic","excerpt":"x"},
   "alternatives":[{"id":"a1","label":"rvc tv only","conditions":[{"kind":"no_shift_required"},{"kind":"rvc","thresholds":{"transaction_value":"62.5"}}]}]},
  {"id":"S-FORB","code":"7708","description":"rvc with forbidden chapter","source":{"citation":"synthetic","excerpt":"x"},
   "alternatives":[{"id":"a1","label":"rvc","conditions":[{"kind":"no_shift_required"},{"kind":"forbidden_inputs","same_as_good_level":"chapter"},{"kind":"rvc","thresholds":{"transaction_value":"60"}}]}]},
  {"id":"S-PARENT","code":"7709","description":"parent heading","source":{"citation":"synthetic","excerpt":"x"},
   "alternatives":[{"id":"a1","label":"heading","conditions":[{"kind":"change_from_heading"}]}]},
  {"id":"S-CHILD","code":"770910","description":"child subheading","source":{"citation":"synthetic","excerpt":"x"},
   "alternatives":[{"id":"a1","label":"chapter","conditions":[{"kind":"change_from_chapter"}]}]},
  {"id":"S-EXCL","code":"7710","description":"heading shift except a sibling","source":{"citation":"synthetic","excerpt":"x"},
   "alternatives":[{"id":"a1","label":"heading except","conditions":[{"kind":"change_from_heading","exclude_prefixes":["771190"]}]}]}
 ]
}`
