export interface RuntimeConfig {
  currentBaseURL?: string;
  embedPolicy?: "any" | "self" | "allowlist" | "deny";
  embedAllowOrigins?: string;
  appURL?: {
    appURLMode?: "path" | "domain" | "dual";
    appDomainSuffix?: string;
    appURLScheme?: "http" | "https";
    appURLPort?: string;
    appPathBase?: string;
  };
  mode?: string;
  version?: string;
  cooldownSeconds?: number;
  anonymousPolicy?: {
    deployLimit?: number;
  };
  limits?: {
    maxSingleFileBytes?: number;
    maxSiteTotalBytes?: number;
    maxFilesPerSite?: number;
  };
  email?: {
    verificationEnabled?: boolean;
    smtpConfigured?: boolean;
  };
  storage?: {
    backend?: string;
    hostedDir?: string;
    ossProvider?: string;
    ossEndpoint?: string;
    ossBucket?: string;
    ossPrefix?: string;
    ossPublicBaseURL?: string;
    ossConfigured?: boolean;
  };
}

export interface SessionInfo {
  success?: boolean;
  userId?: string;
  username?: string;
  label?: string;
  isAdmin?: boolean;
  loginMethod?: string;
}

export interface MarketplaceDeploy {
  id?: string;
  publicId?: string;
  code: string;
  title?: string;
  description?: string;
  filename?: string;
  filePath?: string;
  status?: string;
  visibility?: string;
  category?: string;
  accessProtected?: boolean;
  owned?: boolean;
  isPinned?: boolean;
  viewCount?: number;
  likeCount?: number;
  favoriteCount?: number;
  favorited?: boolean;
  versionCount?: number;
  fileSize?: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface DeployResponse {
  success: boolean;
  id?: string;
  code: string;
  url: string;
  detailUrl?: string;
  versionUrl?: string;
  versionNumber?: number;
  versionId?: string;
  category?: string;
  size?: number;
}

export interface MarketCategoryInfo {
  slug: string;
  label: string;
  note?: string;
}

export interface ScreenInfo {
  id: string;
  name?: string;
  status?: string;
  currentSiteCode?: string;
  currentVersion?: number;
  deviceInfo?: unknown;
  runtime?: string;
  lastHeartbeatAt?: string;
}

export interface DeployFilePayload {
  path: string;
  content?: string;
  contentBase64?: string;
}
