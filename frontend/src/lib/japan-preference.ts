// The preferential agreement codes are shared with the Japan Customs corpus.
// Saved cases and API responses may name any corpus agreement.
export const japanAgreements: Record<string, string> = {
  "JP-EU EPA": "Japan–EU agreement",
  CPTPP: "CPTPP",
  AJCEP: "ASEAN–Japan agreement (AJCEP)",
  RCEP: "RCEP",
  "JP-US TA": "Japan–U.S. agreement",
  GSP: "Generalized System of Preferences (GSP)",
  "JP-India CEPA": "Japan–India agreement",
  "JP-Thailand EPA": "Japan–Thailand agreement",
  "JP-Indonesia EPA": "Japan–Indonesia agreement",
  "JP-Australia EPA": "Japan–Australia agreement",
  "JP-UK CEPA": "Japan–UK agreement",
  "JP-Vietnam EPA": "Japan–Vietnam agreement",
  "JP-Malaysia EPA": "Japan–Malaysia agreement",
  "JP-Mexico EPA": "Japan–Mexico agreement",
  "JP-Philippines EPA": "Japan–Philippines agreement",
  "JP-Switzerland EPA": "Japan–Switzerland agreement",
  "JP-Chile EPA": "Japan–Chile agreement",
};

// The new-case selector offers only agreements with embedded rule schedules.
export const japanTariffEditions: Record<string, string> = {
  "JP-EU EPA": "2017",
  CPTPP: "2012",
  AJCEP: "2017",
  RCEP: "2022",
};

export const supportedJapanAgreements = Object.keys(japanTariffEditions);

export function validJapanHS(value: string): boolean {
  return /^[0-9]{4}(?:[. ]?[0-9]{2}(?:[-. ]?[0-9]{3})?)?$/.test(value.trim());
}
