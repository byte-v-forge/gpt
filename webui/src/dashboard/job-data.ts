import { objectValue } from '@byte-v-forge/common-ui';
import type { JobData } from './types';

export function jobDataObject(data: JobData | Record<string, unknown> | null | undefined): Record<string, unknown> {
  const raw = objectValue(data);
  const entry = firstObjectEntry(raw);
  if (!entry) return {};
  const [key, value] = entry;
  return key === 'activity_progress' ? progressObject(value) : value;
}

function progressObject(data: Record<string, unknown>) {
  const fields = objectValue(objectValue(data.progress).fields);
  const nested = firstObjectEntry(fields)?.[1];
  return nested ? { ...data, ...nested } : data;
}

function firstObjectEntry(data: Record<string, unknown>): [string, Record<string, unknown>] | undefined {
  for (const [key, value] of Object.entries(data)) {
    const nested = objectValue(value);
    if (Object.keys(nested).length > 0) return [key, nested];
  }
  return undefined;
}
