import { allCountries } from 'country-region-data';
import type { SelectFieldOption } from '@byte-v-forge/common-ui';

const preferredRegionByCountry: Record<string, string> = {
  ID: 'ID-JK',
  JP: 'JP-13',
  SG: 'SG-01',
  TH: 'TH-10',
  US: 'US-NY',
};

const countryDisplay = typeof Intl.DisplayNames === 'function'
  ? new Intl.DisplayNames([browserLocale()], { type: 'region', fallback: 'code' })
  : null;

export const countryOptions: SelectFieldOption[] = allCountries
  .map(([name, code]) => ({ value: normalizeCountryCode(code), label: countryLabel(code, name) }))
  .sort((left, right) => String(left.label).localeCompare(String(right.label)));

export function regionOptionsForCountry(countryCode: string): SelectFieldOption[] {
  const country = countryData(countryCode);
  if (!country) return [];
  const normalizedCountry = normalizeCountryCode(country[1]);
  return country[2]
    .map(([name, code]) => regionOption(normalizedCountry, name, code))
    .filter((option): option is SelectFieldOption => option !== null);
}

export function randomRegionForCountry(countryCode: string) {
  const country = normalizeCountryCode(countryCode);
  const options = regionOptionsForCountry(country);
  const preferred = preferredRegionByCountry[country];
  const candidates = options.length ? options : preferred ? [{ value: preferred, label: preferred }] : [];
  if (!candidates.length) return '';
  return candidates[Math.floor(Math.random() * candidates.length)]?.value ?? '';
}

function countryData(countryCode: string) {
  const country = normalizeCountryCode(countryCode);
  return allCountries.find((item) => normalizeCountryCode(item[1]) === country);
}

function regionOption(countryCode: string, name: string, code: string): SelectFieldOption | null {
  const regionCode = normalizeRegionCode(countryCode, code);
  if (!regionCode) return null;
  return { value: regionCode, label: `${name} (${regionCode})` };
}

function countryLabel(code: string, fallback: string) {
  const country = normalizeCountryCode(code);
  const label = countryDisplay?.of(country) || fallback;
  return `${label} (${country})`;
}

function normalizeCountryCode(value: string) {
  return value.trim().toUpperCase();
}

function normalizeRegionCode(countryCode: string, value: string) {
  const region = value.trim().toUpperCase();
  if (!region) return '';
  if (region.includes('-')) return region;
  return `${normalizeCountryCode(countryCode)}-${region}`;
}

function browserLocale() {
  return globalThis.navigator?.language || 'en-US';
}
