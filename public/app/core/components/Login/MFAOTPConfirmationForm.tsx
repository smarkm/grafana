import { css } from '@emotion/css';
import { useId } from 'react';
import { useForm } from 'react-hook-form';

import { GrafanaTheme2 } from '@grafana/data';
import { selectors } from '@grafana/e2e-selectors';
import { t, Trans } from '@grafana/i18n';
import { Button, Input, Field, useStyles2 } from '@grafana/ui';

export interface MFAOTPFormModel {
  otpCode: string;
}

interface Props {
  email?: string;
  onSubmit: (data: MFAOTPFormModel) => void;
  isLoggingIn: boolean;
}

export const MFAOTPConfirmation = ({ email, onSubmit, isLoggingIn }: Props) => {
  const styles = useStyles2(getStyles);
  const otpCodeId = useId();

  const {
    handleSubmit,
    register,
    formState: { errors },
  } = useForm<MFAOTPFormModel>({ mode: 'onChange' });

  return (
    <div className={styles.wrapper}>
      <p className={styles.description}>
        <Trans i18nKey="login.mfa-otp.description" values={{ email: email || 'your email' }}>
          We sent a verification code to your email. Enter it below to complete login.
        </Trans>
      </p>
      <form onSubmit={handleSubmit(onSubmit)}>
        <Field
          label={t('login.form.otp-code-label', 'Verification code')}
          invalid={!!errors.otpCode}
          error={errors.otpCode?.message}
        >
          <Input
            {...register('otpCode', {
              required: t('login.form.otp-code-required', 'Verification code is required'),
            })}
            id={otpCodeId}
            autoFocus
            autoCapitalize="none"
            autoComplete="one-time-code"
            placeholder={t('login.form.otp-code-placeholder', '123456')}
            data-testid={selectors.pages.Login.submit}
          />
        </Field>
        <Button type="submit" className={styles.submitButton} disabled={isLoggingIn}>
          {isLoggingIn ? t('login.form.submit-loading-label', 'Logging in...') : t('login.form.submit-label', 'Log in')}
        </Button>
      </form>
    </div>
  );
};

const getStyles = (theme: GrafanaTheme2) => ({
  wrapper: css({
    width: '100%',
    paddingBottom: theme.spacing(2),
  }),
  description: css({
    marginBottom: theme.spacing(2),
  }),
  submitButton: css({
    justifyContent: 'center',
    width: '100%',
  }),
});
