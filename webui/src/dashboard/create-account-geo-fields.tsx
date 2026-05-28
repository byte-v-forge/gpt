import { ControlledSelectField } from '@byte-v-forge/common-ui';
import type { Control } from '@byte-v-forge/common-ui';
import type { CreateAccountValues } from './create-account-options';
import { countryOptions, regionOptionsForCountry } from './geo-options';

export function CreateAccountGeoFields({ control, regionOptions }: {
  control: Control<CreateAccountValues>;
  regionOptions: ReturnType<typeof regionOptionsForCountry>;
}) {
  return (
    <>
      <ControlledSelectField
        control={control}
        name="country_code"
        label="国家"
        inputId="create-account-country-code"
        options={countryOptions}
      />
      <ControlledSelectField
        control={control}
        name="region"
        label="地区"
        inputId="create-account-region"
        placeholder="未配置地区"
        options={regionOptions}
      />
    </>
  );
}
