package otapi

// --- Catalog / Categories ---

type CatalogResponse struct {
	ErrorCode string `json:"ErrorCode"`
	Result    struct {
		Roots []Category `json:"Roots"`
	} `json:"Result"`
}

type Category struct {
	ID           string `json:"Id"`
	ProviderType string `json:"ProviderType"`
	ExternalID   string `json:"ExternalId"`
	Name         string `json:"Name"`
	IsParent     bool   `json:"IsParent"`
	IsHidden     bool   `json:"IsHidden"`
	IsVirtual    bool   `json:"IsVirtual"`
	IsInternal   bool   `json:"IsInternal"`
	ItemCount    int    `json:"ItemCount"`
}

// --- Search ---

type SearchResponse struct {
	ErrorCode string `json:"ErrorCode"`
	RequestID string `json:"RequestId"`
	Result    struct {
		Items struct {
			Items struct {
				Content    []SearchItem `json:"Content"`
				TotalCount int          `json:"TotalCount"`
			} `json:"Items"`
			MaximumPageCount int    `json:"MaximumPageCount"`
			Provider         string `json:"Provider"`
		} `json:"Items"`
	} `json:"Result"`
}

type SearchItem struct {
	ID               string    `json:"Id"`
	ProviderType     string    `json:"ProviderType"`
	Title            string    `json:"Title"`
	OriginalTitle    string    `json:"OriginalTitle"`
	CategoryID       string    `json:"CategoryId"`
	ExternalCategory string    `json:"ExternalCategoryId"`
	VendorID         string    `json:"VendorId"`
	VendorName       string    `json:"VendorName"`
	VendorScore      int       `json:"VendorScore"`
	BrandID          string    `json:"BrandId"`
	BrandName        string    `json:"BrandName"`
	MainPictureURL   string    `json:"MainPictureUrl"`
	TaobaoItemURL    string    `json:"TaobaoItemUrl"`
	StuffStatus      string    `json:"StuffStatus"`
	Volume           int       `json:"Volume"`
	MasterQuantity   int       `json:"MasterQuantity"`
	IsSellAllowed    bool      `json:"IsSellAllowed"`
	Price            Price     `json:"Price"`
	Pictures         []Picture `json:"Pictures"`
	Features         []string  `json:"Features"`
	FeaturedValues   []KV      `json:"FeaturedValues"`
	Location         *Location `json:"Location"`
}

type Location struct {
	City  string `json:"City"`
	State string `json:"State"`
}

// --- Product Detail ---

type ProductResponse struct {
	ErrorCode string      `json:"ErrorCode"`
	Result    ProductItem `json:"Result"`
}

type ProductItem struct {
	ID                        string          `json:"Id"`
	ProviderType              string          `json:"ProviderType"`
	Title                     string          `json:"Title"`
	OriginalTitle             string          `json:"OriginalTitle"`
	CategoryID                string          `json:"CategoryId"`
	ExternalCategoryID        string          `json:"ExternalCategoryId"`
	VendorID                  string          `json:"VendorId"`
	VendorName                string          `json:"VendorName"`
	VendorDisplayName         string          `json:"VendorDisplayName"`
	VendorScore               int             `json:"VendorScore"`
	BrandID                   string          `json:"BrandId"`
	BrandName                 string          `json:"BrandName"`
	MainPictureURL            string          `json:"MainPictureUrl"`
	TaobaoItemURL             string          `json:"TaobaoItemUrl"`
	ExternalItemURL           string          `json:"ExternalItemUrl"`
	StuffStatus               string          `json:"StuffStatus"`
	Volume                    int             `json:"Volume"`
	MasterQuantity            int             `json:"MasterQuantity"`
	IsSellAllowed             bool            `json:"IsSellAllowed"`
	SellDisallowReason        string          `json:"SellDisallowReason"`
	HasHierarchicalConf       bool            `json:"HasHierarchicalConfigurators"`
	FirstLotQuantity          int             `json:"FirstLotQuantity"`
	NextLotQuantity           int             `json:"NextLotQuantity"`
	Price                     Price           `json:"Price"`
	Pictures                  []Picture       `json:"Pictures"`
	Attributes                []Attribute     `json:"Attributes"`
	ConfiguredItems           []SKU           `json:"ConfiguredItems"`
	Description               string          `json:"Description"`
	Features                  []string        `json:"Features"`
	FeaturedValues            []KV            `json:"FeaturedValues"`
	LastUpdatedTime           string          `json:"LastUpdatedTime"`
	PhysicalParameters        *PhysicalParams `json:"PhysicalParameters"`
	Location                  *Location       `json:"Location"`
}

type Attribute struct {
	Pid          string `json:"Pid"`
	Vid          string `json:"Vid"`
	PropertyName string `json:"PropertyName"`
	Value        string `json:"Value"`
	IsConfigurator bool  `json:"IsConfigurator"`
	ImageURL     string `json:"ImageUrl"`
}

type SKU struct {
	ID           string        `json:"Id"`
	Quantity     int           `json:"Quantity"`
	SalesCount   int           `json:"SalesCount"`
	Configurators []Configurator `json:"Configurators"`
	Price        Price         `json:"Price"`
}

type Configurator struct {
	Pid string `json:"Pid"`
	Vid string `json:"Vid"`
}

type Price struct {
	OriginalPrice              float64 `json:"OriginalPrice"`
	MarginPrice                float64 `json:"MarginPrice"`
	OriginalCurrencyCode       string  `json:"OriginalCurrencyCode"`
}

type Picture struct {
	URL    string       `json:"Url"`
	Small  PictureSize  `json:"Small"`
	Medium PictureSize  `json:"Medium"`
	Large  PictureSize  `json:"Large"`
	IsMain bool         `json:"IsMain"`
}

type PictureSize struct {
	URL    string `json:"Url"`
	Width  int    `json:"Width"`
	Height int    `json:"Height"`
}

type PhysicalParams struct {
	Weight float64 `json:"Weight"`
	Length float64 `json:"Length"`
	Width  float64 `json:"Width"`
	Height float64 `json:"Height"`
}

type KV struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}
