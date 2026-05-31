import { useState, useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  FormControl, InputLabel, Select, MenuItem, TextField, Chip,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Switch, FormControlLabel, Collapse, IconButton, Alert,
  Accordion, AccordionSummary, AccordionDetails, Grid,
  Dialog, DialogTitle, DialogContent, DialogActions,
  List, ListItemButton, ListItemText,
  InputAdornment, Paper, ClickAwayListener,
} from '@mui/material'
import { PlayArrow, ExpandMore, ExpandLess, Info, Add, Close, Search } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { SyncJob, Category } from '../types'

function jobColor(s: string): any {
  return s === 'done' ? 'success' : s === 'running' ? 'info' : s === 'error' ? 'error' : 'default'
}
function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}
function dur(start: number, end: number) {
  if (!start || !end) return '-'
  const s = end - start
  if (s < 60) return `${s}с`
  return `${Math.floor(s / 60)}м ${s % 60}с`
}

function JobLogPanel({ jobId }: { jobId: number }) {
  const [open, setOpen] = useState(false)
  const { data } = useQuery({
    queryKey: ['sync-job', jobId],
    queryFn: () => api.get(`/sync/jobs/${jobId}`).then(r => r.data.data),
    refetchInterval: open ? 2_000 : false,
    enabled: open,
  })
  return (
    <Box>
      <IconButton size="small" onClick={() => setOpen(v => !v)}>
        {open ? <ExpandLess sx={{ fontSize: 14 }} /> : <ExpandMore sx={{ fontSize: 14 }} />}
      </IconButton>
      <Collapse in={open}>
        <Box sx={{ background: '#1a1a2e', color: '#a8ff78', fontFamily: 'monospace', fontSize: 11, p: 1.5, mt: 0.5, borderRadius: 1, maxHeight: 200, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
          {data?.log || 'Лог пуст'}
        </Box>
      </Collapse>
    </Box>
  )
}

const ORDER_BY_OPTIONS = [
  { value: '',                 label: 'По умолчанию (релевантность)' },
  { value: 'Volume:Desc',      label: 'По продажам (больше → меньше)' },
  { value: 'Price:Asc',        label: 'По цене (дешевле → дороже)' },
  { value: 'Price:Desc',       label: 'По цене (дороже → дешевле)' },
  { value: 'UpdatedTime:Desc', label: 'Сначала новые' },
]

// Brand autocomplete with dropdown
function BrandInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [open, setOpen] = useState(false)
  const [input, setInput] = useState(value)
  const ref = useRef<HTMLDivElement>(null)

  const { data: brands = [] } = useQuery<{ name: string; count: number }[]>({
    queryKey: ['sync-brands', input],
    queryFn: () => api.get('/sync/brands', { params: { q: input } }).then(r => r.data.data),
    enabled: open,
  })

  useEffect(() => { setInput(value) }, [value])

  return (
    <ClickAwayListener onClickAway={() => setOpen(false)}>
      <Box ref={ref} sx={{ position: 'relative' }}>
        <TextField
          size="small"
          fullWidth
          label="Бренд"
          value={input}
          onChange={e => { setInput(e.target.value); onChange(e.target.value); setOpen(true) }}
          onFocus={() => setOpen(true)}
          helperText="Начни вводить или выбери из списка известных брендов"
          slotProps={{
            input: {
              endAdornment: input ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={() => { setInput(''); onChange('') }}>
                    <Close fontSize="small" />
                  </IconButton>
                </InputAdornment>
              ) : null,
            },
          }}
        />
        {open && brands.length > 0 && (
          <Paper sx={{ position: 'absolute', zIndex: 1300, width: '100%', mt: 0.5, maxHeight: 220, overflow: 'auto', boxShadow: 4 }}>
            {brands.map(b => (
              <ListItemButton
                key={b.name}
                dense
                onClick={() => { setInput(b.name); onChange(b.name); setOpen(false) }}
              >
                <ListItemText
                  primary={b.name}
                  slotProps={{ primary: { sx: { fontSize: 13 } } }}
                />
                <Typography sx={{ fontSize: 11, color: 'text.secondary', ml: 1 }}>{b.count} товаров</Typography>
              </ListItemButton>
            ))}
          </Paper>
        )}
      </Box>
    </ClickAwayListener>
  )
}

// Property search builder: pid:value pairs
interface PropFilter { pid: string; pidLabel: string; vid: string; value: string; valueLabel: string }

function PropertySearchBuilder({ value, onChange }: {
  value: string
  onChange: (v: string) => void
}) {
  const [filters, setFilters] = useState<PropFilter[]>([])
  const [dialogOpen, setDialogOpen] = useState(false)
  const [step, setStep] = useState<'prop' | 'value'>('prop')
  const [selectedProp, setSelectedProp] = useState<{ pid: string; label: string } | null>(null)
  const [propSearch, setPropSearch] = useState('')
  const [valueSearch, setValueSearch] = useState('')

  const { data: props = [] } = useQuery<{ pid: string; label: string; count: number }[]>({
    queryKey: ['sync-properties'],
    queryFn: () => api.get('/sync/properties').then(r => r.data.data),
  })

  const { data: vals = [] } = useQuery<{ vid: string; value: string; label: string; count: number }[]>({
    queryKey: ['sync-prop-values', selectedProp?.pid],
    queryFn: () => api.get('/sync/properties', { params: { pid: selectedProp!.pid } }).then(r => r.data.data),
    enabled: !!selectedProp && step === 'value',
  })

  const filteredProps = props.filter(p =>
    !propSearch || p.label.toLowerCase().includes(propSearch.toLowerCase()) || p.pid.includes(propSearch)
  )
  const filteredVals = vals.filter(v =>
    !valueSearch || v.label.toLowerCase().includes(valueSearch.toLowerCase()) || v.value.includes(valueSearch)
  )

  const applyFilters = (list: PropFilter[]) => {
    setFilters(list)
    // PropertySearch format: pid:vid (using vid as value identifier for 1688)
    const str = list.map(f => `${f.pid}:${f.vid}`).join(';')
    onChange(str)
  }

  const removeFilter = (idx: number) => {
    const next = filters.filter((_, i) => i !== idx)
    applyFilters(next)
  }

  const openDialog = () => {
    setStep('prop')
    setSelectedProp(null)
    setPropSearch('')
    setValueSearch('')
    setDialogOpen(true)
  }

  const selectProp = (pid: string, label: string) => {
    setSelectedProp({ pid, label })
    setValueSearch('')
    setStep('value')
  }

  const selectValue = (vid: string, val: string, valLabel: string) => {
    const already = filters.findIndex(f => f.pid === selectedProp!.pid && f.vid === vid)
    if (already >= 0) { setDialogOpen(false); return }
    const next = [...filters, { pid: selectedProp!.pid, pidLabel: selectedProp!.label, vid, value: val, valueLabel: valLabel }]
    applyFilters(next)
    setDialogOpen(false)
  }

  return (
    <Box>
      <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 0.5 }}>
        Фильтр по свойствам товара
      </Typography>
      <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', mb: filters.length > 0 ? 1 : 0 }}>
        {filters.map((f, i) => (
          <Chip
            key={i}
            label={`${f.pidLabel}: ${f.valueLabel}`}
            size="small"
            color="primary"
            variant="outlined"
            onDelete={() => removeFilter(i)}
          />
        ))}
      </Stack>
      <Button size="small" variant="outlined" startIcon={<Add />} onClick={openDialog}>
        Добавить фильтр по свойству
      </Button>
      {value && (
        <Typography sx={{ fontSize: 10, color: 'text.disabled', mt: 0.5, fontFamily: 'monospace' }}>
          → {value}
        </Typography>
      )}

      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ pb: 1 }}>
          {step === 'prop' ? 'Выбери свойство' : `Выбери значение для "${selectedProp?.label}"`}
        </DialogTitle>
        <DialogContent sx={{ p: 0 }}>
          <Box sx={{ px: 2, py: 1 }}>
            <TextField
              size="small"
              fullWidth
              autoFocus
              placeholder={step === 'prop' ? 'Поиск свойства...' : 'Поиск значения...'}
              value={step === 'prop' ? propSearch : valueSearch}
              onChange={e => step === 'prop' ? setPropSearch(e.target.value) : setValueSearch(e.target.value)}
              slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }}
            />
          </Box>
          <List dense sx={{ maxHeight: 320, overflow: 'auto' }}>
            {step === 'prop' && filteredProps.map(p => (
              <ListItemButton key={p.pid} onClick={() => selectProp(p.pid, p.label)}>
                <ListItemText
                  primary={p.label}
                  secondary={p.pid !== p.label ? p.pid : undefined}
                  slotProps={{ primary: { sx: { fontSize: 13 } }, secondary: { sx: { fontSize: 10 } } }}
                />
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{p.count} тов.</Typography>
              </ListItemButton>
            ))}
            {step === 'value' && filteredVals.map(v => (
              <ListItemButton key={v.vid} onClick={() => selectValue(v.vid, v.value, v.label)}>
                <ListItemText
                  primary={v.label}
                  secondary={v.value !== v.label ? v.value : undefined}
                  slotProps={{ primary: { sx: { fontSize: 13 } }, secondary: { sx: { fontSize: 10 } } }}
                />
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{v.count} тов.</Typography>
              </ListItemButton>
            ))}
          </List>
        </DialogContent>
        <DialogActions>
          {step === 'value' && (
            <Button size="small" onClick={() => setStep('prop')}>Назад</Button>
          )}
          <Button size="small" onClick={() => setDialogOpen(false)}>Отмена</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default function Sync() {
  const [searchParams] = useSearchParams()
  const qc = useQueryClient()

  const [categoryId, setCategoryId]     = useState(searchParams.get('category') ?? '')
  const [maxProducts, setMaxProducts]   = useState('500')
  const [pricesOnly, setPricesOnly]     = useState(false)

  const [minPrice, setMinPrice]         = useState('')
  const [maxPrice, setMaxPrice]         = useState('')
  const [maxPriceLimit, setMaxPriceLimit] = useState('')

  const [itemTitle, setItemTitle]             = useState('')
  const [vendorName, setVendorName]           = useState('')
  const [brandName, setBrandName]             = useState('')
  const [propertySearch, setPropertySearch]   = useState('')
  const [minVolume, setMinVolume]             = useState('')
  const [orderBy, setOrderBy]                 = useState('')
  const [minVendorRating, setMinVendorRating] = useState('')
  const [maxVendorRating, setMaxVendorRating] = useState('')
  const [firstLotMin, setFirstLotMin]         = useState('')
  const [firstLotMax, setFirstLotMax]         = useState('')
  const [featureComplete, setFeatureComplete] = useState(false)
  const [featureDiscount, setFeatureDiscount] = useState(false)
  const [featureTmall, setFeatureTmall]       = useState(false)

  const [histStatus, setHistStatus]   = useState('')
  const [histJobType, setHistJobType] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['sync'],
    queryFn: () => api.get('/sync').then(r => r.data.data),
    refetchInterval: 5_000,
  })

  const runSync = useMutation({
    mutationFn: () => api.post('/sync/run', {
      category_id:      categoryId,
      max_products:     parseInt(maxProducts) || 500,
      min_price:        parseFloat(minPrice) || 0,
      max_price:        parseFloat(maxPrice) || 0,
      max_price_limit:  parseFloat(maxPriceLimit) || 0,
      prices_only:      pricesOnly,
      item_title:       itemTitle,
      vendor_name:      vendorName,
      brand_name:       brandName,
      property_search:  propertySearch,
      min_volume:       parseInt(minVolume) || 0,
      order_by:         orderBy,
      stuff_status:     'New',
      min_vendor_rating: parseInt(minVendorRating) || 0,
      max_vendor_rating: parseInt(maxVendorRating) || 0,
      first_lot_min:    parseInt(firstLotMin) || 0,
      first_lot_max:    parseInt(firstLotMax) || 0,
      feature_complete: featureComplete,
      feature_discount: featureDiscount,
      feature_tmall:    featureTmall,
    }),
    onSuccess: r => { toast.success(`Задача #${r.data.data.job_id} запущена`); qc.invalidateQueries({ queryKey: ['sync'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const jobs: SyncJob[] = data?.jobs ?? []
  const cats: Category[] = data?.categories ?? []
  const filteredJobs = jobs.filter(j => {
    if (histStatus && j.Status !== histStatus) return false
    if (histJobType && j.JobType !== histJobType) return false
    return true
  })
  const runningJob = jobs.find(j => j.Status === 'running')

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Синхронизация</Typography>

      {runningJob && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Задача #{runningJob.ID} выполняется... Таблица обновляется каждые 5 сек.
        </Alert>
      )}

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: 2 }}>Запустить синхронизацию</Typography>
          <Stack spacing={2}>

            <FormControlLabel
              control={<Switch checked={pricesOnly} onChange={e => setPricesOnly(e.target.checked)} />}
              label={
                <Box>
                  <Typography sx={{ fontSize: 13, fontWeight: 500 }}>
                    {pricesOnly ? 'Только цены' : 'Полная синхронизация товаров'}
                  </Typography>
                  <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                    {pricesOnly
                      ? 'Обновляет цены и наличие уже загруженных товаров. Быстро.'
                      : 'Загружает новые товары из 1688, обновляет описания, атрибуты, фото.'}
                  </Typography>
                </Box>
              }
            />

            <Grid container spacing={2}>
              <Grid size={{ xs: 12, md: 6 }}>
                <FormControl size="small" fullWidth>
                  <InputLabel>Категория</InputLabel>
                  <Select value={categoryId} label="Категория" onChange={e => setCategoryId(e.target.value as string)}>
                    <MenuItem value="">Все включённые категории</MenuItem>
                    {cats.filter(c => c.Enabled).map(c => (
                      <MenuItem key={c.ID} value={c.ID}>
                        {c.Name}
                        <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>({c.ID})</Typography>
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <TextField size="small" fullWidth label="Макс. товаров" type="number"
                  value={maxProducts} onChange={e => setMaxProducts(e.target.value)}
                  helperText="Сколько товаров загрузить максимум" />
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <FormControl size="small" fullWidth>
                  <InputLabel>Сортировка API</InputLabel>
                  <Select value={orderBy} label="Сортировка API" onChange={e => setOrderBy(e.target.value)}>
                    {ORDER_BY_OPTIONS.map(o => <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>)}
                  </Select>
                </FormControl>
              </Grid>
            </Grid>

            <Box>
              <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary', mb: 1 }}>Фильтр по цене (CNY)</Typography>
              <Alert severity="info" icon={<Info fontSize="small" />} sx={{ py: 0.5, mb: 1.5, '& .MuiAlert-message': { fontSize: 12 } }}>
                Мин/макс цена - фильтрует что запрашивать у 1688 API.
                Лимит аномалий - пропускает товары дороже этой суммы (защита от выбросов). 0 = отключено.
              </Alert>
              <Stack direction="row" spacing={2} sx={{ flexWrap: 'wrap' }}>
                <TextField size="small" label="Мин. цена CNY" type="number"
                  value={minPrice} onChange={e => setMinPrice(e.target.value)}
                  sx={{ width: 150 }} helperText="0 = без минимума" />
                <TextField size="small" label="Макс. цена CNY" type="number"
                  value={maxPrice} onChange={e => setMaxPrice(e.target.value)}
                  sx={{ width: 150 }} helperText="0 = без максимума" />
                <TextField size="small" label="Лимит аномалий CNY" type="number"
                  value={maxPriceLimit} onChange={e => setMaxPriceLimit(e.target.value)}
                  sx={{ width: 175 }} helperText="Пропустить товары дороже N" placeholder="напр. 3000" />
              </Stack>
            </Box>

            {!pricesOnly && (
              <Accordion disableGutters elevation={0} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: '8px !important', '&:before': { display: 'none' } }}>
                <AccordionSummary expandIcon={<ExpandMore />}>
                  <Typography sx={{ fontSize: 13, fontWeight: 500 }}>Расширенные фильтры 1688</Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Stack spacing={2.5}>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Поиск товаров</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 12, md: 4 }}>
                        <TextField size="small" fullWidth label="Название товара" value={itemTitle}
                          onChange={e => setItemTitle(e.target.value)}
                          helperText="Ключевые слова на 1688 (китайский или английский)" />
                      </Grid>
                      <Grid size={{ xs: 12, md: 4 }}>
                        <BrandInput value={brandName} onChange={setBrandName} />
                      </Grid>
                      <Grid size={{ xs: 12, md: 4 }}>
                        <PropertySearchBuilder value={propertySearch} onChange={setPropertySearch} />
                      </Grid>
                    </Grid>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Продавец</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 12, md: 4 }}>
                        <TextField size="small" fullWidth label="Имя продавца" value={vendorName}
                          onChange={e => setVendorName(e.target.value)}
                          helperText="Фильтр по имени продавца" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Мин. рейтинг продавца" type="number"
                          value={minVendorRating} onChange={e => setMinVendorRating(e.target.value)}
                          helperText="Рекомендуется 8+" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Макс. рейтинг продавца" type="number"
                          value={maxVendorRating} onChange={e => setMaxVendorRating(e.target.value)}
                          helperText="Обычно не нужен (0 = без ограничения)" />
                      </Grid>
                    </Grid>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Объём и лот (только 1688)</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <TextField size="small" fullWidth label="Мин. продаж" type="number"
                          value={minVolume} onChange={e => setMinVolume(e.target.value)}
                          helperText="Рекомендуется 50+" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <TextField size="small" fullWidth label="Мин. первый лот" type="number"
                          value={firstLotMin} onChange={e => setFirstLotMin(e.target.value)}
                          helperText="Мин. кол-во в заказе. Обычно 1" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <TextField size="small" fullWidth label="Макс. первый лот" type="number"
                          value={firstLotMax} onChange={e => setFirstLotMax(e.target.value)}
                          helperText="Рекомендуется 10 (отсекает оптовые)" />
                      </Grid>
                    </Grid>

                    <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap' }}>
                      <FormControlLabel
                        control={<Switch size="small" checked={featureComplete} onChange={e => setFeatureComplete(e.target.checked)} />}
                        label={
                          <Box>
                            <Typography sx={{ fontSize: 12 }}>Feature Complete</Typography>
                            <Typography sx={{ fontSize: 10, color: 'text.secondary' }}>Только полностью заполненные карточки</Typography>
                          </Box>
                        }
                      />
                      <FormControlLabel
                        control={<Switch size="small" checked={featureDiscount} onChange={e => setFeatureDiscount(e.target.checked)} />}
                        label={<Typography sx={{ fontSize: 12 }}>Только со скидкой</Typography>}
                      />
                      <FormControlLabel
                        control={<Switch size="small" checked={featureTmall} onChange={e => setFeatureTmall(e.target.checked)} />}
                        label={
                          <Box>
                            <Typography sx={{ fontSize: 12 }}>Только Tmall</Typography>
                            <Typography sx={{ fontSize: 10, color: 'text.secondary' }}>Официальные магазины на 1688</Typography>
                          </Box>
                        }
                      />
                    </Stack>
                  </Stack>
                </AccordionDetails>
              </Accordion>
            )}

            <Box>
              <Button variant="contained" size="large" startIcon={<PlayArrow />}
                onClick={() => runSync.mutate()} disabled={runSync.isPending || !!runningJob}>
                {runningJob ? 'Задача уже выполняется...' : 'Запустить'}
              </Button>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <Stack direction="row" spacing={2} sx={{ mb: 2, alignItems: 'center', flexWrap: 'wrap' }}>
            <Typography sx={{ fontWeight: 600 }}>История задач</Typography>
            <Box sx={{ flex: 1 }} />
            <FormControl size="small" sx={{ minWidth: 130 }}>
              <InputLabel>Тип</InputLabel>
              <Select value={histJobType} label="Тип" onChange={e => setHistJobType(e.target.value)}>
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="products">Товары</MenuItem>
                <MenuItem value="prices">Цены</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 130 }}>
              <InputLabel>Статус</InputLabel>
              <Select value={histStatus} label="Статус" onChange={e => setHistStatus(e.target.value)}>
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="done">Завершено</MenuItem>
                <MenuItem value="running">Выполняется</MenuItem>
                <MenuItem value="error">Ошибка</MenuItem>
              </Select>
            </FormControl>
          </Stack>

          {isLoading && <LinearProgress />}

          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>#</TableCell>
                  <TableCell>Тип</TableCell>
                  <TableCell>Категория</TableCell>
                  <TableCell>Статус</TableCell>
                  <TableCell align="right">Загружено</TableCell>
                  <TableCell align="right">Пропущено</TableCell>
                  <TableCell align="right">Ошибок</TableCell>
                  <TableCell align="right">API запр.</TableCell>
                  <TableCell>Начало</TableCell>
                  <TableCell>Время</TableCell>
                  <TableCell>Лог</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {filteredJobs.map(job => (
                  <TableRow key={job.ID} hover sx={job.Status === 'running' ? { bgcolor: 'action.hover' } : {}}>
                    <TableCell sx={{ color: 'text.secondary', fontSize: 12 }}>{job.ID}</TableCell>
                    <TableCell>
                      <Chip
                        label={job.JobType === 'prices' ? 'Цены' : 'Товары'}
                        size="small"
                        variant="outlined"
                        color={job.JobType === 'prices' ? 'warning' : 'primary'}
                      />
                    </TableCell>
                    <TableCell><Typography sx={{ fontSize: 12 }}>{job.CategoryID || 'все'}</Typography></TableCell>
                    <TableCell><Chip label={job.Status} color={jobColor(job.Status)} size="small" /></TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, fontWeight: 600, color: (job.ItemsProcessed ?? 0) > 0 ? 'success.main' : 'text.disabled' }}>
                        {(job.ItemsProcessed ?? 0) > 0 ? `+${job.ItemsProcessed}` : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>
                        {(job.ItemsSkipped ?? 0) > 0 ? job.ItemsSkipped : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, color: (job.ErrorsCount ?? 0) > 0 ? 'error.main' : 'text.secondary' }}>
                        {(job.ErrorsCount ?? 0) > 0 ? job.ErrorsCount : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                        {(job.APIRequests ?? 0) > 0 ? job.APIRequests : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell><Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{ts(job.StartedAt)}</Typography></TableCell>
                    <TableCell><Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{dur(job.StartedAt, job.FinishedAt)}</Typography></TableCell>
                    <TableCell><JobLogPanel jobId={job.ID} /></TableCell>
                  </TableRow>
                ))}
                {filteredJobs.length === 0 && !isLoading && (
                  <TableRow>
                    <TableCell colSpan={11} align="center" sx={{ py: 4, color: 'text.secondary' }}>
                      Нет задач по выбранным фильтрам
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </Box>
  )
}
