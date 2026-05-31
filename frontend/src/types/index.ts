// Field names match Go struct JSON serialization (PascalCase)

export interface DashboardStats {
  TotalProducts: number
  TotalCategories: number
  EnabledCategories: number
  MappedCategories: number
  PendingPush: number
  PendingTranslate: number
  PushedProducts: number
  LastSync: SyncJob | null
}

export interface SyncJob {
  ID: number
  JobType: string
  CategoryID: string
  Status: string
  TriggeredBy: string
  ItemsProcessed: number
  ItemsSkipped: number
  ErrorsCount: number
  APIRequests: number
  StartedAt: number
  FinishedAt: number
  Log: string
}

export interface Category {
  ID: string
  Provider: string
  Name: string
  NameEn: string
  NameZh: string
  ParentID: string
  IsParent: boolean
  Enabled: boolean
  ItemCount: number
  LocalCount: number
  CSCategoryID: number
  CSCategoryName: string
  ItemCountM: string
  ItemCountK: string
  Children?: Category[]
}

export interface Product {
  ID: number
  CategoryID: string
  Provider: string
  OtapiID: string
  TitleOriginal: string
  TitleRu: string
  TitleEn: string
  TitleTk: string
  DescriptionRU: string
  PriceCNY: number
  PriceTMT: number
  MainImageURL: string
  LocationState: string
  LocationCity: string
  LocationStateRu: string
  LocationCityRu: string
  VolumeSales: number
  TranslateStatus: string
  Enabled: boolean
  PushedToCsAt: number
  CsProductID: number
  UpdatedAt: number
}

export interface CategoryMapping {
  OTCategoryID: string
  CSCategoryID: number
  CSCategoryName: string
  Notes: string
  WeightG: number
  MinPriceCNY: number
  MaxPriceCNY: number
  MinVolume: number
}

export interface CSCartCategory {
  CategoryID: number
  ParentID: number
  Name: string
}

export interface AttrTranslation {
  pid: string
  vid: string
  property_name_zh: string
  value_zh: string
  property_name_ru: string
  value_ru: string
  translated_at: number
}
