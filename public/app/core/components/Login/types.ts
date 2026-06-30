export interface LoginDTO {
  message: string;
  redirectUrl?: string;
  otpRequired?: boolean;
  otpToken?: string;
  email?: string;
}

export interface AuthNRedirectDTO {
  URL: string;
}
