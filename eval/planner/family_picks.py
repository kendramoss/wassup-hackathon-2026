# Per-family plan checks (D171): the rulings each fixture is built from,
# picked by the rule in docs/DECISIONS.md D171 before any plan search ran.
# (family, ruling, split, how it was picked)
TEMPLATE_FILE = {'motorised_tools_small_appliances': 'motorised_tools'}
PICKS = [
 ('pcba_electronic_devices', 'N350727', 'dev', 'China-held'),
 ('pcba_electronic_devices', 'N352230', 'dev', 'China-held'),
 ('vehicle_parts', 'N349871', 'dev', 'China-held'),
 ('vehicle_parts', 'N352073', 'dev', 'China-held'),
 ('cables_harnesses', 'N350058', 'dev', 'China-held'),
 ('cables_harnesses', 'N350769', 'dev', 'China-held'),
 ('motorised_tools_small_appliances', 'N350310', 'dev', 'China-held'),
 ('motorised_tools_small_appliances', 'N351922', 'dev', 'China-held; N352061 skipped as the same product as N350310'),
 ('power_supplies_chargers', 'N352355', 'dev', 'China-held'),
 ('power_supplies_chargers', 'N350383', 'dev', 'no second China-held ruling; earliest other'),
 ('hand_tools', 'N352429', 'dev', 'China-held'),
 ('hand_tools', 'N354220', 'dev', 'China-held'),
 ('led_lights', 'N335263', 'train', 'no dev China-held ruling; train China-held'),
 ('led_lights', 'H304679', 'train', 'no dev China-held ruling; train China-held'),
 ('medical_devices', 'N333453', 'train', 'no dev China-held ruling; train China-held'),
 ('medical_devices', 'N350152', 'dev', 'no second China-held ruling; earliest dev other'),
 ('water_filters', 'H339955', 'train', 'no dev China-held ruling; train China-held'),
 ('water_filters', 'N350826', 'dev', 'no second China-held ruling; earliest dev other'),
 ('display_camera_modules', 'N352223', 'dev', 'no China-held ruling; earliest dev'),
 ('display_camera_modules', 'N354175', 'dev', 'no China-held ruling; second earliest dev'),
 ('furniture', 'N350678', 'dev', 'no China-held ruling; earliest dev'),
 ('furniture', 'H345343', 'dev', 'no China-held ruling; second earliest dev'),
 ('gas_grills_cooking', 'N354448', 'dev', 'the only eligible ruling'),
]
