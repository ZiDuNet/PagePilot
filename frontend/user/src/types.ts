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
  cooldownSeconds?: number;
  anonymousPolicy?: {
    deployLimit?: number;
  };
  limits?: {
    maxSingleFileBytes?: number;
    maxSiteTotalBytes?: number;
    maxFilesPerSite?: number;
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
  accessProtected?: boolean;
  owned?: boolean;
  isPinned?: boolean;
  viewCount?: number;
  likeCount?: number;
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
  size?: number;
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
