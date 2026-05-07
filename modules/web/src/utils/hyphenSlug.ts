import type { Rule } from 'antd/es/form';

/** Must stay in sync with modules/internal/naming/hyphen_slug.go hyphenSlugPattern. */
export const HYPHEN_SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/** Matches gorm Certificate.Name size */
export const MAX_CERTIFICATE_NAME_LEN = 191;

/** Matches gorm Role.Name size */
export const MAX_ROLE_NAME_LEN = 64;

const HYPHEN_SLUG_MESSAGE =
  '仅允许小写字母与数字，分段之间使用英文连字符 -；不允许空格、下划线或大写字母';

export function hyphenSlugFormRules(
  maxLen: number,
  opts?: { requiredMessage?: string; exemptNormalizedValue?: string | null }
): Rule[] {
  const exempt = opts?.exemptNormalizedValue?.trim();
  const shapeRule: Rule =
    exempt != null && exempt !== ''
      ? {
          validator: async (_rule, value: string) => {
            const v = (value || '').trim();
            if (!v) return;
            if (v === exempt) return;
            if (!HYPHEN_SLUG_PATTERN.test(v)) {
              return Promise.reject(new Error(HYPHEN_SLUG_MESSAGE));
            }
          }
        }
      : { pattern: HYPHEN_SLUG_PATTERN, message: HYPHEN_SLUG_MESSAGE };

  return [
    { required: true, message: opts?.requiredMessage ?? '请输入名称' },
    { max: maxLen, message: `最多 ${maxLen} 个字符` },
    shapeRule
  ];
}
