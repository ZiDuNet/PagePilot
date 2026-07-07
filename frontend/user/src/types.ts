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
  registrationAllowed?: boolean;
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
  tags?: string[];
  accessProtected?: boolean;
  owned?: boolean;
  canManage?: boolean;
  isPinned?: boolean;
  viewCount?: number;
  likeCount?: number;
  reuseCount?: number;
  templateSourceCode?: string;
  templateSourceVersion?: number;
  favoriteCount?: number;
  favorited?: boolean;
  versionCount?: number;
  fileSize?: number;
  createdAt?: string;
  updatedAt?: string;
  bundle?: BundleDetail;
  files?: ContentFile[];
  reuse?: ReuseDetail;
}

export interface BundleDetail {
  kind?: string;
  kindLabel?: string;
  root?: string;
  mainEntry?: string;
  securityMode?: string;
  siteSecurityMode?: string;
  effectiveSecurityMode?: string;
  fileCount?: number;
  totalSize?: number;
  tree?: BundleTreeItem[];
  entryNote?: string;
}

export interface BundleTreeItem {
  path: string;
  size?: number;
  isBinary?: boolean;
  sha256?: string;
}

export interface ContentFile {
  path: string;
  size?: number;
  sha256?: string;
  isBinary?: boolean;
}

export interface ReuseDetail {
  downloadUrl?: string;
  detailUrl?: string;
  cli?: string;
  agentPrompt?: string;
  mcp?: Record<string, unknown>;
  allowReuse?: boolean;
  allowDownload?: boolean;
  policyNote?: string;
  templateSourceCode?: string;
  templateSourceVersion?: number;
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
  reuseCount?: number;
  templateSourceCode?: string;
  templateSourceVersion?: number;
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
