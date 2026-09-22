export const en: Record<string, string> = {
  // Header
  'app.title': 'MrW POE2 Filter',
  'header.settings': 'Settings',
  'header.back': 'Back',
  'header.hide': 'Hide panel',
  'save.saving': 'saving…',
  'save.saved': 'saved',
  'save.error': 'could not save',

  // Status card
  'status.starting': 'Starting…',
  'status.updating': 'Updating',
  'status.failed': 'Filter update failed',
  'status.fresh': 'Filter up to date',
  'status.never': 'Not updated yet',
  'status.lastUpdate': 'Last update {0}',
  'status.dirty': 'Settings changed. Update to apply them to the filter.',
  'status.writeFailed':
    'The filter in the game folder could not be written, {0}. New settings reach the game only after a successful update.',
  'status.fileFrom': 'the file is still the one written {0}',
  'status.fileNever': 'it has never been written',
  'button.updating': 'Updating…',
  'button.retry': 'Try again',
  'button.updateNow': 'Update now',
  'meta.autoRetry': 'Auto retry: {0}',
  'meta.next': 'Next: {0}',
  'meta.autoOff': 'Automatic update is off',
  'meta.reload': 'In game: Item Filter → Reload',

  // Threshold
  'threshold.title': 'Value threshold',
  'threshold.hide': 'Items below this value are hidden.',
  'threshold.dim': 'Items below this value are dimmed.',
  'threshold.show': 'Items below this value are still shown.',

  // Base filter
  'base.title': 'NeverSink base',
  'base.soft': 'Soft',
  'base.strict': 'Strict',
  'base.uber': 'Uber+',
  'mode.title': 'Items below the threshold',
  'mode.hide': 'Hide',
  'mode.dim': 'Dim',
  'mode.show': 'Show',

  // Exceptional scan
  'scan.toggle': 'Exceptional scan',
  'scan.hint': 'Prices bases with extra sockets or 21%+ quality from trade',
  'scan.scanned': '{0} / {1} scanned',
  'scan.valuable': '{0} valuable',
  'scan.next': 'Next: {0}',
  'scan.searching': 'searching…',
  'scan.last': 'Last: {0}',
  'scan.note':
    'Searches run about every {0} s to protect the quota. First pass over every base ≈ {1}; the bases NeverSink rates highly go first.',

  // Summary
  'stats.currency': 'currency',
  'stats.uniqueBases': 'unique bases',
  'stats.exceptional': 'exceptional',
  'stats.caption': 'highlighted above the threshold · 1 div = {0} ex',

  // Gear
  'gear.title': 'Equipment',
  'gear.strict': 'Strict equipment filter',
  'gear.strictHint': 'Hides ordinary weapons and armour',
  'gear.quality': 'Show high quality gear',
  'gear.qualityOff': 'Off',

  // Custom rules
  'tier.off': 'None',
  'tier.hide': 'Hide',
  'tier.t5rare': 'Unidentified rare equipment',
  'tier.t5rareHint': 'Hide all · None: NeverSink decides · a tier: shown from there up',
  'tier.jewels': 'Rare jewels',
  'tier.jewelsHint': 'Hide all · None: NeverSink decides · a tier: shown from there up',
  'tier.uncut': 'Uncut skill and spirit gems',
  'tier.uncutHint': 'Hide all · None: NeverSink decides · a level: shown from there up',
  'tier.support': 'Uncut support gems',
  'tier.supportHint': 'They drop constantly, so they have their own stop',
  'tier.waystones': 'Waystones',
  'tier.waystonesHint': 'Hide all · None: NeverSink decides · a tier: highlighted from there up',

  'rules.title': 'Custom rules',
  'rules.pinnacle': 'Highlight pinnacle keys',
  'rules.hideExalt': 'Hide Exalted Orbs',
  'rules.hideGold': 'Hide Gold',

  // Lists
  'lists.title': 'Lists',
  'lists.showTop': 'Always show — spotlight',
  'lists.showTopNote': 'strong highlight outside value tiers',
  'lists.showTopDesc':
    'An item in a value tier uses that tier’s look; everything else in this list gets a strong highlight. Valuable unique bases are already handled automatically.',
  'lists.showMid': 'Always show — medium',
  'lists.showMidNote': 'never hidden, medium highlight',
  'lists.showMidDesc':
    'An item that is already valuable keeps its strong highlight; otherwise it gets this medium look.',
  'lists.hide': 'Always hide',
  'lists.chance': 'Chance bases',
  'lists.chanceNote': 'normal rarity only',
  'lists.searchItem': 'Search a unique, currency or base…',
  'lists.searchHide': 'Search an item to hide…',
  'lists.searchBase': 'Search a base or unique…',

  // My groups
  'groups.title': 'My groups',
  'groups.desc':
    'Your own lists and value tiers. Every visible group has its own colour and sound, set under Appearance.',
  'groups.order': 'Order sets priority for Show/Hide groups; value groups are automatically ordered by threshold.',
  'groups.reorder': 'Move group',
  'groups.duplicates': 'Also in another list: {0}',
  'groups.add': 'Add group',
  'groups.namePlaceholder': 'Group name',
  'groups.modeShow': 'Show',
  'groups.modeHide': 'Hide',
  'groups.modeValue': 'Value threshold',
  'groups.valueDesc': 'Items worth at least this amount use the highest value group whose threshold they reach.',
  'groups.valueAmount': 'Group threshold',
  'groups.valueEquivalent': 'Current equivalent: {0}',
  'groups.valueTooLow': 'This group is not used: {0} must be above the base threshold of {1}.',
  'groups.always': 'Always win',
  'groups.alwaysHint': 'Off: an item worth more than the threshold keeps its stronger highlight',
  'groups.delete': 'Delete',
  'groups.deleteConfirm': 'Delete?',
  'groups.empty': 'No groups yet.',
  'groups.limit': 'At most {0} groups.',
  'groups.lookLink': 'Colour and sound: Appearance → {0}',

  // Appearance
  'look.title': 'Appearance',
  'look.desc':
    "Pick a colour and a sound per group: the app's own themes, NeverSink's styles or your own colours.",
  'look.applyNeverSink': 'Use NeverSink colours everywhere',
  'look.reset': 'Reset to default',
  'look.customised': 'Customised',
  'look.sound': 'Sound',
  'look.soundDefault': 'Default ({0})',
  'look.soundDefaultGame': 'game sound {0}',
  'look.soundDefaultSilent': 'silent',
  'look.soundNone': 'Silent',
  'look.soundGame': 'Game sound {0}',
  'look.play': 'Play',
  'look.gameSoundNote':
    'All 26 game sounds belong to Path of Exile; 17–26 are the ones it plays for currency drops. The game plays them itself, and the copy here is only for listening.',
  'look.playAria': 'Play the sound',
  'look.addSound': 'Add your own sound…',
  'look.soundHint':
    'An mp3 or wav of your own is copied into the filter folder, where the game reads it from. Keep it short: it plays on every matching drop.',

  // Theme picker
  'picker.appThemes': 'App themes',
  'picker.nsThemes': 'NeverSink themes',
  'picker.nsEmpty': 'NeverSink styles appear after the first update.',
  'picker.default': 'Default',
  'picker.tabApp': 'App',
  'picker.custom': 'Custom',
  'picker.background': 'Background',
  'picker.text': 'Text',
  'picker.border': 'Border',
  'picker.beam': 'Beam',
  'picker.icon': 'Icon',
  'picker.shape': 'Shape',
  'picker.none': 'None',
  'picker.noShape': 'No shape',
  'picker.noBeam': 'No beam',

  // Preview
  'preview.aria': '{0} preview',
  'preview.sample': 'Sample item',
  'preview.beam': 'Beam: {0}',
  'preview.beamTemp': 'Beam: {0} (temporary)',
  'preview.noBeam': 'No beam',
  'preview.minimap': 'Minimap: {0} {1}',
  'preview.noMinimap': 'No minimap icon',
  'preview.soundSuffix': ' · Sound: {0}',

  // Auto update
  'auto.title': 'Automatic update',
  'auto.enable': 'Update on start and at a regular interval',
  'auto.hours': '{0} h',
  'auto.notify': 'Show a notification after each update',

  // Trade scan
  'trade.title': 'Trade scan',
  'trade.desc': 'The trade search quota (600 / 6 hours) is shared with your own searches on the trade site.',
  'trade.budget': '{0}%',

  // General
  'share.title': 'Share scan results',
  'share.desc':
    'A full scan costs hours of rate-limited searches. Send the file to a friend so they start with your prices instead of scanning from zero.',
  'share.export': 'Export',
  'share.import': 'Import',
  'share.exported': 'Saved: {0}',
  'share.imported': '{0} new, {1} updated, {2} already newer here.',

  // Profiles
  'profile.title': 'Profiles',
  'profile.desc':
    'A profile is a complete settings set. Keep one per kind of farming and switch with a click; the filter is rewritten right away.',
  'profile.namePlaceholder': 'New profile name',
  'profile.saveAs': 'Save as',
  'profile.delete': 'Delete',
  'profile.deleteConfirm': 'Delete?',
  'profile.export': 'Export',
  'profile.import': 'Import',
  'profile.switched': 'Switched to {0}.',
  'profile.imported': '{0} imported and loaded.',
  'profile.exported': 'Saved: {0}',
  'profile.filterNameChanged': 'This profile writes {0}.filter — pick it in game with Item Filter.',
  'profile.leagueChanged': 'The league is now {0}.',
  'profile.everything':
    'A profile carries every setting, league and filter name included, which is what makes the file shareable.',
  'filter.export': 'Export the filter file',
  'filter.exportHint': 'A copy of the filter that was last written, for sharing or another machine',

  'general.title': 'General',
  'general.language': 'Language',
  'general.languageAuto': 'System language ({0})',
  'general.league': 'League',
  'general.leagueUnlisted':
    'This league is not in the current list; your choice is kept, pick a new one if you like.',
  'general.filterName': 'Filter name in game',
  'general.filterNameChanged':
    'The filter is now written as {0}.filter, but the game may still have {1} selected. Pick {0} in Options → Item Filter, then Reload.',
  'general.customBase': 'Custom base filter (NeverSink when empty)',
  'general.customBasePlaceholder': 'C:\\…\\myfilter.filter',
  'general.priceServer': 'Price server (future)',

  // Footer actions
  'actions.filterFolder': 'Filter folder',
  'actions.dataFolder': 'Data folder',
  'actions.quit': 'Quit',
  'footer.testMode': ' · test mode',

  // List editor
  'editor.uniqueOnly': 'Unique only',
  'editor.uniqueOnlyTop': 'Unique only · best {0}',
  'editor.allRarities': 'All rarities',
  'editor.remove': 'remove {0}',

  // Relative time
  'time.never': 'never',
  'time.justNow': 'just now',
  'time.minsAgo': '{0} min ago',
  'time.hoursAgo': '{0} h {1} min ago',
  'time.secs': '{0} s',
  'time.mins': '{0} min',
  'time.hours': '{0} h {1} min',

  // Minimap icon shapes and colours
  'shape.Star': 'Star',
  'shape.Diamond': 'Diamond',
  'shape.Circle': 'Circle',
  'shape.Square': 'Square',
  'shape.Triangle': 'Triangle',
  'shape.Hexagon': 'Hexagon',
  'shape.Pentagon': 'Pentagon',
  'shape.Cross': 'Cross',
  'shape.Kite': 'Kite',
  'shape.UpsideDownHouse': 'Upside-down house',
  'colour.Blue': 'Blue',
  'colour.Brown': 'Brown',
  'colour.Cyan': 'Cyan',
  'colour.Green': 'Green',
  'colour.Grey': 'Grey',
  'colour.Orange': 'Orange',
  'colour.Pink': 'Pink',
  'colour.Purple': 'Purple',
  'colour.Red': 'Red',
  'colour.White': 'White',
  'colour.Yellow': 'Yellow',
}
