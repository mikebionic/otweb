import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  TextField, Select, MenuItem, FormControl, IconButton, Chip, Tabs, Tab, Autocomplete,
  Collapse, Tooltip,
} from '@mui/material'
import { Add, Delete, ExpandMore, ExpandLess, CheckCircle, AutoAwesome } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { CategoryMapping, CSCartCategory } from '../types'

export default function Mapping() {
  const [tab, setTab] = useState(0)
  const [otCat, setOtCat] = useState('')
  const [csCat, setCsCat] = useState('')
  const [csCatName, setCsCatName] = useState('')
  const [notes, setNotes] = useState('')
  const [filterUnmapped, setFilterUnmapped] = useState(false)
  const [filterPending, setFilterPending] = useState(false)
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['mapping'],
    queryFn: () => api.get('/mapping').then(r => r.data.data),
  })

  const addMapping = useMutation({
    mutationFn: () => api.post('/mapping/add', {
      ot_category_id: otCat,
      cs_category_id: parseInt(csCat),
      cs_category_name: csCatName,
      notes,
    }),
    onSuccess: () => {
      toast.success('Маппинг добавлен')
      setOtCat(''); setCsCat(''); setCsCatName(''); setNotes('')
      qc.invalidateQueries({ queryKey: ['mapping'] })
    },
    onError: () => toast.error('Ошибка'),
  })

  const deleteMapping = useMutation({
    mutationFn: (id: string) => api.post('/mapping/delete', { ot_category_id: id }),
    onSuccess: () => { toast.success('Удалено'); qc.invalidateQueries({ queryKey: ['mapping'] }) },
  })

  const mappings: CategoryMapping[] = data?.mappings ?? []
  const unmappedOT = data?.unmapped_ot ?? []
  const csCategories: CSCartCategory[] = data?.cs_categories ?? []

  const { data: attrData } = useQuery({
    queryKey: ['attr-mapping'],
    queryFn: () => api.get('/attrs/mapping').then(r => r.data.data),
    enabled: tab === 1,
  })

  const setAttrFeature = useMutation({
    mutationFn: (req: { pid: string; cs_feature_id: number }) => api.post('/attrs/set-feature', req),
    onSuccess: () => { toast.success('Сохранено'); qc.invalidateQueries({ queryKey: ['attr-mapping'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const acceptSuggest = useMutation({
    mutationFn: (pid: string) => api.post('/attrs/accept-suggest', { pid }),
    onSuccess: () => { toast.success('Принято'); qc.invalidateQueries({ queryKey: ['attr-mapping'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const verifyAttr = useMutation({
    mutationFn: ({ pid, verified }: { pid: string; verified: number }) =>
      api.post('/attrs/verify', { pid, verified }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['attr-mapping'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const runSuggest = useMutation({
    mutationFn: () => api.post('/attrs/run-suggest', {}),
    onSuccess: () => { toast.success('AI suggest запущен'); setTimeout(() => qc.invalidateQueries({ queryKey: ['attr-mapping'] }), 3000) },
  })

  type AttrRow = {
    pid: string; name_ru: string; name_zh: string
    cs_feature_id: number; cs_feature_name: string; cs_feature_type: string
    values_total: number; values_mapped: number
    suggest_feature_id: number; suggest_score: number; suggest_feature_name: string
    verified: number
  }
  type CSFeatureItem = { feature_id: number; name: string }
  const attrListRaw: AttrRow[] = attrData?.attrs ?? []
  const csFeatures: CSFeatureItem[] = attrData?.cs_features ?? []

  const attrList = attrListRaw.filter(a => {
    if (filterUnmapped && a.cs_feature_id > 0) return false
    if (filterPending && a.suggest_feature_id === 0 && a.cs_feature_id === 0) return false
    return true
  })

  const totalAttrs = attrListRaw.length
  const mappedAttrs = attrListRaw.filter(a => a.cs_feature_id > 0).length
  const pendingSuggest = attrListRaw.filter(a => a.cs_feature_id === 0 && a.suggest_feature_id > 0).length
  const verified = attrListRaw.filter(a => a.verified > 0).length

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: '4px' }}>Mapping</Typography>
      <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 2 }}>
        OT Commerce → CS-Cart. Только замапленные категории синхронизируются на магазин.
      </Typography>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
        <Tab label="Категории" />
        <Tab label="Атрибуты" />
      </Tabs>

      {tab === 0 && (<>
      {isLoading && <LinearProgress />}

      {mappings.length > 0 && (
        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography sx={{ fontWeight: 600, mb: 2 }}>
              Активные маппинги <Chip label={mappings.length} color="success" size="small" sx={{ ml: 0.5 }} />
            </Typography>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>OT категория</TableCell>
                    <TableCell>CS-Cart категория</TableCell>
                    <TableCell>Вес (г)</TableCell>
                    <TableCell>MOQ</TableCell>
                    <TableCell>Цена CNY (min-max)</TableCell>
                    <TableCell>Мин. продаж</TableCell>
                    <TableCell>Ключевое слово → кат.</TableCell>
                    <TableCell>Пол → категория</TableCell>
                    <TableCell></TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {mappings.map(m => (
                    <TableRow key={m.OTCategoryID} hover>
                      <TableCell>
                        <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{m.OTCategoryPath || m.OTCategoryName || m.OTCategoryID}</Typography>
                        <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{m.OTCategoryID}</Typography>
                      </TableCell>
                      <TableCell>
                        <Chip label={m.CSCategoryPath || m.CSCategoryName} color="success" size="small" />
                        <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>#{m.CSCategoryID}</Typography>
                      </TableCell>
                      <TableCell>
                        <InlineInput value={m.WeightG} placeholder="г"
                          onSave={v => api.post('/mapping/set-weight', { ot_category_id: m.OTCategoryID, weight_g: v }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))} />
                      </TableCell>
                      <TableCell>
                        <InlineInput value={m.MOQ} placeholder="1"
                          onSave={v => api.post('/mapping/set-moq', { ot_category_id: m.OTCategoryID, moq: v }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))} />
                      </TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                          <InlineInput value={m.MinPriceCNY} placeholder="min"
                            onSave={v => api.post('/mapping/set-filters', { ot_category_id: m.OTCategoryID, min_price_cny: v, max_price_cny: m.MaxPriceCNY, min_volume: m.MinVolume }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))} />
                          <Typography sx={{ color: 'text.disabled' }}>-</Typography>
                          <InlineInput value={m.MaxPriceCNY} placeholder="max"
                            onSave={v => api.post('/mapping/set-filters', { ot_category_id: m.OTCategoryID, min_price_cny: m.MinPriceCNY, max_price_cny: v, min_volume: m.MinVolume }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))} />
                        </Stack>
                      </TableCell>
                      <TableCell>
                        <InlineInput value={m.MinVolume} placeholder="0=все"
                          onSave={v => api.post('/mapping/set-filters', { ot_category_id: m.OTCategoryID, min_price_cny: m.MinPriceCNY, max_price_cny: m.MaxPriceCNY, min_volume: v }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))} />
                      </TableCell>
                      <TableCell>
                        <InlineTextInput
                          keyword={m.TitleKeyword}
                          altCatId={m.AltCSCategoryID}
                          onSave={(kw, altId) => api.post('/mapping/set-keyword', {
                            ot_category_id: m.OTCategoryID,
                            title_keyword: kw,
                            alt_cs_category_id: altId,
                          }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))}
                        />
                      </TableCell>
                      <TableCell>
                        <Stack spacing={0.5}>
                          <Select size="small" variant="standard" value={m.CSCategoryMale || 0}
                            onChange={e => api.post('/mapping/set-gender-cats', { ot_category_id: m.OTCategoryID, cs_category_male: Number(e.target.value), cs_category_female: m.CSCategoryFemale || 0 }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))}
                            sx={{ fontSize: 11, minWidth: 160 }}>
                            <MenuItem value={0} sx={{ fontSize: 11 }}><em>♂ базовая</em></MenuItem>
                            {csCategories.map(c => <MenuItem key={c.CategoryID} value={c.CategoryID} sx={{ fontSize: 11 }}>♂ {c.Path || c.Name}</MenuItem>)}
                          </Select>
                          <Select size="small" variant="standard" value={m.CSCategoryFemale || 0}
                            onChange={e => api.post('/mapping/set-gender-cats', { ot_category_id: m.OTCategoryID, cs_category_male: m.CSCategoryMale || 0, cs_category_female: Number(e.target.value) }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))}
                            sx={{ fontSize: 11, minWidth: 160 }}>
                            <MenuItem value={0} sx={{ fontSize: 11 }}><em>♀ базовая</em></MenuItem>
                            {csCategories.map(c => <MenuItem key={c.CategoryID} value={c.CategoryID} sx={{ fontSize: 11 }}>♀ {c.Path || c.Name}</MenuItem>)}
                          </Select>
                        </Stack>
                      </TableCell>
                      <TableCell>
                        <IconButton size="small" color="error"
                          onClick={() => { if (confirm(`Удалить маппинг ${m.OTCategoryID}?`)) deleteMapping.mutate(m.OTCategoryID) }}>
                          <Delete fontSize="small" />
                        </IconButton>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: 2 }}>Добавить маппинг</Typography>
          <Stack spacing={2}>
            <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} sx={{ alignItems: 'flex-start' }}>
              <Box sx={{ flex: 1 }}>
                <Typography sx={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', color: 'text.secondary', mb: '4px', letterSpacing: '0.05em' }}>
                  OT Commerce — откуда берём товары
                </Typography>
                <FormControl size="small" fullWidth>
                  <Select value={otCat} displayEmpty onChange={e => setOtCat(e.target.value as string)}>
                    <MenuItem value=""><em>— выбери OT категорию —</em></MenuItem>
                    {unmappedOT.filter((c: any) => !c.IsHeader).map((c: any) => (
                      <MenuItem key={c.ID} value={c.ID}>
                        <Typography component="span" sx={{ fontSize: 13 }}>{c.Path || c.Name}</Typography>
                        {c.ItemCount > 0 && (
                          <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>
                            ({c.ItemCount >= 1000 ? Math.round(c.ItemCount/1000)+'K' : c.ItemCount} тов.)
                          </Typography>
                        )}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Typography sx={{ fontSize: 11, color: 'text.secondary', mt: '4px' }}>Незамапленных: {unmappedOT.filter((c: any) => !c.IsHeader).length}</Typography>
              </Box>

              <Box sx={{ alignSelf: 'center', pt: 2 }}>
                <Typography sx={{ fontSize: 22, color: 'text.disabled' }}>→</Typography>
              </Box>

              <Box sx={{ flex: 1 }}>
                <Typography sx={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', color: 'text.secondary', mb: '4px', letterSpacing: '0.05em' }}>
                  CS-Cart магазин — куда загружаем
                </Typography>
                <FormControl size="small" fullWidth>
                  <Select
                    value={csCat}
                    displayEmpty
                    onChange={e => {
                      const val = e.target.value as string
                      setCsCat(val)
                      const found = csCategories.find(c => String(c.CategoryID) === val)
                      if (found) setCsCatName(found.Name)
                    }}
                  >
                    <MenuItem value=""><em>— выбери CS-Cart категорию —</em></MenuItem>
                    {csCategories.map(c => (
                      <MenuItem key={c.CategoryID} value={String(c.CategoryID)}
                        sx={{ opacity: c.Status === 'H' ? 0.55 : 1 }}>
                        <Box>
                          <Typography sx={{ fontSize: 13 }}>
                            {c.Path || c.Name}
                            {c.Status === 'H' && <Typography component="span" sx={{ fontSize: 10, color: 'text.disabled', ml: 0.5 }}>(скрытая)</Typography>}
                          </Typography>
                          <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>#{c.CategoryID}</Typography>
                        </Box>
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Typography component="a" onClick={() => api.post('/mapping/refresh-cscart').then(() => { toast.success('CS-Cart обновлён'); qc.invalidateQueries({ queryKey: ['mapping'] }) })}
                  sx={{ fontSize: 11, color: 'primary.main', cursor: 'pointer', mt: '4px', display: 'block' }}>
                  Обновить список CS-Cart
                </Typography>
              </Box>

              <Box sx={{ flex: 0.5, pt: 2 }}>
                <TextField size="small" fullWidth label="Заметка (необяз.)" value={notes} onChange={e => setNotes(e.target.value)} placeholder="напр.: женские сумки" />
              </Box>
            </Stack>

            <Box>
              <Button variant="contained" startIcon={<Add />}
                disabled={!otCat || !csCat || addMapping.isPending}
                onClick={() => addMapping.mutate()}>
                Добавить маппинг
              </Button>
            </Box>
          </Stack>
        </CardContent>
      </Card>
      </>)}

      {tab === 1 && (<>
        {/* Статистика и управление */}
        <Stack direction="row" spacing={1.5} sx={{ mb: 2, flexWrap: 'wrap', alignItems: 'center' }}>
          <Chip label={`Всего: ${totalAttrs}`} size="small" variant="outlined" />
          <Chip label={`Замаплено: ${mappedAttrs}`} size="small" color="success" variant="outlined" />
          <Chip label={`AI suggest: ${pendingSuggest}`} size="small" color="warning" variant="outlined"
            icon={<AutoAwesome sx={{ fontSize: 14 }} />} />
          <Chip label={`Верифицировано: ${verified}`} size="small" color="info" variant="outlined"
            icon={<CheckCircle sx={{ fontSize: 14 }} />} />
          <Box sx={{ flex: 1 }} />
          <Button size="small" variant={filterUnmapped ? 'contained' : 'outlined'}
            onClick={() => setFilterUnmapped(v => !v)}>
            Только незамапленные
          </Button>
          <Button size="small" variant={filterPending ? 'contained' : 'outlined'}
            onClick={() => setFilterPending(v => !v)}>
            Есть AI suggest
          </Button>
          <Button size="small" variant="outlined" startIcon={<AutoAwesome />}
            onClick={() => runSuggest.mutate()} disabled={runSuggest.isPending}>
            Запустить AI suggest
          </Button>
        </Stack>

        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <TableContainer sx={{ maxHeight: 700 }}>
              <Table size="small" stickyHeader>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ width: 32 }}></TableCell>
                    <TableCell sx={{ width: 220 }}>OT атрибут</TableCell>
                    <TableCell sx={{ width: 110 }}>Значений</TableCell>
                    <TableCell>CS-Cart характеристика</TableCell>
                    <TableCell sx={{ width: 130 }}>Статус</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {attrList.map(a => (
                    <AttrRow
                      key={a.pid}
                      row={a}
                      features={csFeatures}
                      onSetFeature={fid => setAttrFeature.mutate({ pid: a.pid, cs_feature_id: fid })}
                      onAcceptSuggest={() => acceptSuggest.mutate(a.pid)}
                      onVerify={v => verifyAttr.mutate({ pid: a.pid, verified: v })}
                    />
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      </>)}
    </Box>
  )
}

function AttrRow({ row, features, onSetFeature, onAcceptSuggest, onVerify }: {
  row: {
    pid: string; name_ru: string; name_zh: string
    cs_feature_id: number; cs_feature_name: string; cs_feature_type: string
    values_total: number; values_mapped: number
    suggest_feature_id: number; suggest_score: number; suggest_feature_name: string
    verified: number
  }
  features: { feature_id: number; name: string }[]
  onSetFeature: (fid: number) => void
  onAcceptSuggest: () => void
  onVerify: (v: number) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const qc = useQueryClient()

  const { data: vidData } = useQuery({
    queryKey: ['attr-vids', row.pid],
    queryFn: () => api.get(`/attrs/${row.pid}/values`, { params: { feature_id: row.cs_feature_id } }).then(r => r.data.data),
    enabled: expanded,
  })

  const setVariant = useMutation({
    mutationFn: (req: { pid: string; vid: string; cs_feature_id: number; cs_variant_id: number }) =>
      api.post('/attrs/set-variant', req),
    onSuccess: () => { toast.success('Значение сохранено'); qc.invalidateQueries({ queryKey: ['attr-vids', row.pid] }) },
    onError: () => toast.error('Ошибка'),
  })

  const acceptValueSuggests = useMutation({
    mutationFn: () => api.post('/attrs/accept-value-suggests', { pid: row.pid }),
    onSuccess: () => { toast.success('Варианты приняты'); qc.invalidateQueries({ queryKey: ['attr-vids', row.pid] }) },
    onError: () => toast.error('Ошибка'),
  })

  const hasSuggest = row.suggest_feature_id > 0 && row.cs_feature_id === 0

  return (
    <>
      <TableRow hover sx={{ '& td': { borderBottom: expanded ? 'none' : undefined } }}>
        <TableCell sx={{ px: 0.5 }}>
          <IconButton size="small" onClick={() => setExpanded(v => !v)}>
            {expanded ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
          </IconButton>
        </TableCell>
        <TableCell>
          <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{row.name_ru || row.pid}</Typography>
          {row.name_ru && <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{row.pid}</Typography>}
        </TableCell>
        <TableCell>
          <Typography sx={{ fontSize: 12 }}>
            {row.values_mapped}/{row.values_total}
            {row.values_mapped < row.values_total && row.cs_feature_id > 0 && (
              <Typography component="span" sx={{ fontSize: 10, color: 'warning.main', ml: 0.5 }}>
                ({row.values_total - row.values_mapped} без маппинга)
              </Typography>
            )}
          </Typography>
        </TableCell>
        <TableCell>
          <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
            <AttrFeatureSelect
              csFeatureId={row.cs_feature_id}
              csFeatureName={row.cs_feature_name}
              features={features}
              onSave={onSetFeature}
            />
            {hasSuggest && (
              <Tooltip title={`AI предлагает: "${row.suggest_feature_name}" (${row.suggest_score}%)`}>
                <Button size="small" variant="outlined" color="warning"
                  startIcon={<AutoAwesome sx={{ fontSize: 14 }} />}
                  onClick={onAcceptSuggest}
                  sx={{ fontSize: 11, py: 0.3, px: 1, whiteSpace: 'nowrap' }}>
                  {row.suggest_feature_name.length > 20
                    ? row.suggest_feature_name.slice(0, 20) + '…'
                    : row.suggest_feature_name}
                  <Typography component="span" sx={{ fontSize: 10, ml: 0.5, opacity: 0.7 }}>
                    {row.suggest_score}%
                  </Typography>
                </Button>
              </Tooltip>
            )}
          </Stack>
        </TableCell>
        <TableCell>
          {row.cs_feature_id > 0 ? (
            row.verified ? (
              <Chip label="Верифицировано" size="small" color="success" icon={<CheckCircle sx={{ fontSize: 14 }} />}
                onClick={() => onVerify(0)} sx={{ cursor: 'pointer', fontSize: 10 }} />
            ) : (
              <Button size="small" variant="outlined" color="success"
                startIcon={<CheckCircle sx={{ fontSize: 14 }} />}
                onClick={() => onVerify(1)} sx={{ fontSize: 11, py: 0.3 }}>
                Верифицировать
              </Button>
            )
          ) : (
            <Typography sx={{ fontSize: 11, color: 'text.disabled' }}>— не замаплено</Typography>
          )}
        </TableCell>
      </TableRow>

      {/* Expanded: vid-level value mapping */}
      <TableRow>
        <TableCell colSpan={5} sx={{ p: 0, border: 0 }}>
          <Collapse in={expanded} unmountOnExit>
            <Box sx={{ pl: 6, pr: 2, py: 1, bgcolor: 'action.hover' }}>
              {!vidData && <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Загрузка...</Typography>}
              {vidData && row.cs_feature_type === 'T' && (
                <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 1, fontStyle: 'italic' }}>
                  Текстовый атрибут — маппинг вариантов не требуется
                </Typography>
              )}
              {vidData && row.cs_feature_type !== 'T' && (vidData.vids ?? []).some((v: any) => v.suggest_variant_id > 0) && (
                <Box sx={{ mb: 1 }}>
                  <Button size="small" variant="outlined" color="warning"
                    startIcon={<AutoAwesome sx={{ fontSize: 14 }} />}
                    onClick={() => acceptValueSuggests.mutate()}
                    sx={{ fontSize: 11, py: 0.3, px: 1 }}>
                    Принять все AI suggest значений
                  </Button>
                </Box>
              )}
              {vidData && (
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell sx={{ fontSize: 11, fontWeight: 600 }}>Значение OT</TableCell>
                      <TableCell sx={{ fontSize: 11, fontWeight: 600 }}>
                        {row.cs_feature_type === 'T' ? 'Текст (авто)' : 'Вариант CS-Cart'}
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {[...(vidData.vids ?? [])].sort((a: any, b: any) => {
                      if (row.cs_feature_type === 'T') return 0
                      if ((a.cs_variant_id > 0) === (b.cs_variant_id > 0)) return 0
                      return a.cs_variant_id > 0 ? 1 : -1
                    }).map((v: any) => {
                      const isTextType = v.cs_feature_type === 'T' || row.cs_feature_type === 'T'
                      const isMapped = isTextType ? v.cs_feature_id > 0 : v.cs_variant_id > 0
                      return (
                      <TableRow key={v.vid} sx={{ bgcolor: !isMapped ? '#fffde7' : undefined }}>
                        <TableCell>
                          <Typography sx={{ fontSize: 12, fontWeight: !isMapped ? 600 : 400 }}>{v.value_ru || v.value_zh || v.vid}</Typography>
                          {v.value_ru && v.value_ru !== v.vid && <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{v.vid}</Typography>}
                        </TableCell>
                        <TableCell>
                          {isTextType ? (
                            <Typography sx={{ fontSize: 11, color: 'success.main' }}>✓ текстовое поле</Typography>
                          ) : (
                          <VidVariantSelect
                            vid={v}
                            variants={vidData.variants ?? []}
                            onSave={(variantId) => setVariant.mutate({
                              pid: v.pid, vid: v.vid,
                              cs_feature_id: row.cs_feature_id,
                              cs_variant_id: variantId,
                            })}
                          />
                          )}
                        </TableCell>
                      </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              )}
            </Box>
          </Collapse>
        </TableCell>
      </TableRow>
    </>
  )
}

const COLOR_HEX: Record<string, string> = {
  'белый': '#FFFFFF', 'чёрный': '#1a1a1a', 'черный': '#1a1a1a',
  'серый': '#9E9E9E', 'светло-серый': '#D3D3D3', 'тёмно-серый': '#555555', 'темно-серый': '#555555', 'антрацит': '#2F2F2F',
  'красный': '#E53935', 'бордовый': '#7B1FA2', 'малиновый': '#C62828', 'фуксия': '#FF4081',
  'розовый': '#F06292', 'светло-розовый': '#FFCDD2', 'пудро-розовый': '#F8BBD0',
  'синий': '#1565C0', 'тёмно-синий': '#0D47A1', 'темно-синий': '#0D47A1', 'голубой': '#64B5F6', 'светло-голубой': '#B3E5FC', 'морской': '#006064', 'индиго': '#3F51B5', 'бирюзовый': '#00BCD4',
  'зеленый': '#43A047', 'зелёный': '#43A047', 'темно-зеленый': '#1B5E20', 'тёмно-зелёный': '#1B5E20', 'мятный': '#80CBC4', 'изумрудный': '#00897B', 'фисташковый': '#AED581',
  'жёлтый': '#FDD835', 'желтый': '#FDD835', 'оранжевый': '#FB8C00', 'горчичный': '#F9A825',
  'фиолетовый': '#7B1FA2', 'сиреневый': '#CE93D8', 'лиловый': '#AB47BC', 'пурпурный': '#8E24AA',
  'коричневый': '#6D4C41', 'бежевый': '#D7CCC8', 'экрю': '#F5F5DC', 'песочный': '#FFCC80', 'ванильный': '#FFF9C4', 'персиковый': '#FFAB91',
  'хаки': '#8D9B6A', 'золотистый': '#FFD54F', 'золотой': '#FFD54F', 'серебряный': '#CFD8DC', 'серебренный': '#CFD8DC',
  'разноцветный': 'linear-gradient(to right, red,orange,yellow,green,blue,indigo,violet)',
  'леопардовый': '#C49A00', 'прозрачный': 'rgba(200,200,200,0.3)',
}

function getColorHex(name: string): string | undefined {
  const lower = name.toLowerCase()
  for (const [key, hex] of Object.entries(COLOR_HEX)) {
    if (lower.includes(key)) return hex
  }
  return undefined
}

function VidVariantSelect({ vid, variants, onSave }: {
  vid: { cs_variant_id: number; variant_value: string; suggest_variant_id: number; suggest_variant_value: string }
  variants: { variant_id: number; value: string }[]
  onSave: (variantId: number) => void
}) {
  const [saved, setSaved] = useState(false)
  const current = vid.cs_variant_id > 0
    ? { variant_id: vid.cs_variant_id, value: vid.variant_value }
    : null

  const handleChange = (_: any, v: { variant_id: number; value: string } | null) => {
    onSave(v ? v.variant_id : 0)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const hex = current ? getColorHex(current.value) : undefined

  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
      {hex && <Box sx={{ width: 16, height: 16, borderRadius: '3px', flexShrink: 0,
        background: hex, border: '1px solid rgba(0,0,0,0.2)' }} />}
      <Autocomplete
        size="small"
        options={variants}
        getOptionLabel={o => o.value}
        isOptionEqualToValue={(o, v) => o.variant_id === v.variant_id}
        value={current}
        onChange={handleChange}
        renderInput={params => <TextField {...params} placeholder="— не выбрано —" sx={{ minWidth: 200 }} />}
        renderOption={(props, o) => {
          const optHex = getColorHex(o.value)
          const { key, ...rest } = props as any
          return (
            <li key={key} {...rest} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '4px 10px' }}>
              <span style={{ width: 14, height: 14, borderRadius: 3, flexShrink: 0, display: 'inline-block',
                background: optHex ?? '#e0e0e0', border: '1px solid rgba(0,0,0,0.15)' }} />
              <span style={{ fontSize: 13 }}>{o.value}</span>
            </li>
          )
        }}
      />
      {saved && <Typography sx={{ fontSize: 11, color: 'success.main', fontWeight: 600 }}>✓ сохранено</Typography>}
      {vid.suggest_variant_id > 0 && vid.cs_variant_id === 0 && (
        <Tooltip title={`AI: "${vid.suggest_variant_value}"`}>
          <Button size="small" variant="outlined" color="warning"
            onClick={() => { onSave(vid.suggest_variant_id); setSaved(true); setTimeout(() => setSaved(false), 2000) }}
            sx={{ fontSize: 10, py: 0.2, px: 0.8, whiteSpace: 'nowrap' }}>
            <AutoAwesome sx={{ fontSize: 12, mr: 0.3 }} />
            {vid.suggest_variant_value.length > 15
              ? vid.suggest_variant_value.slice(0, 15) + '…'
              : vid.suggest_variant_value}
          </Button>
        </Tooltip>
      )}
    </Stack>
  )
}

function AttrFeatureSelect({ csFeatureId, csFeatureName, features, onSave }: {
  csFeatureId: number
  csFeatureName: string
  features: { feature_id: number; name: string }[]
  onSave: (fid: number) => void
}) {
  const current = csFeatureId > 0 ? { feature_id: csFeatureId, name: csFeatureName } : null
  return (
    <Autocomplete
      size="small"
      options={features}
      getOptionLabel={o => o.name}
      isOptionEqualToValue={(o, v) => o.feature_id === v.feature_id}
      value={current}
      onChange={(_, v) => onSave(v ? v.feature_id : 0)}
      renderInput={params => <TextField {...params} placeholder="— не выбрано —" sx={{ minWidth: 250 }} />}
      renderOption={(props, o) => (
        <li {...props} key={o.feature_id}>
          <Typography sx={{ fontSize: 13 }}>{o.name}</Typography>
          <Typography sx={{ fontSize: 10, color: 'text.disabled', ml: 0.5 }}>#{o.feature_id}</Typography>
        </li>
      )}
    />
  )
}

function InlineTextInput({ keyword, altCatId, onSave }: { keyword: string; altCatId: number; onSave: (kw: string, altId: number) => void }) {
  const [kw, setKw] = useState(keyword || '')
  const [alt, setAlt] = useState(String(altCatId || ''))
  return (
    <Stack direction="row" spacing={0.3} sx={{ alignItems: 'center' }}>
      <TextField size="small" value={kw} onChange={e => setKw(e.target.value)} placeholder="слово" sx={{ width: 70 }} />
      <Typography sx={{ color: 'text.disabled', fontSize: 12 }}>→</Typography>
      <TextField size="small" type="number" value={alt} onChange={e => setAlt(e.target.value)} placeholder="catID" sx={{ width: 65 }} />
      <Button size="small" variant="outlined" onClick={() => onSave(kw, parseInt(alt) || 0)} sx={{ minWidth: 28, px: '2px' }}>✓</Button>
    </Stack>
  )
}

function InlineInput({ value, placeholder, onSave }: { value: number; placeholder: string; onSave: (v: number) => void }) {
  const [v, setV] = useState(String(value || ''))
  return (
    <Stack direction="row" spacing={0.3}>
      <TextField size="small" type="number" value={v} onChange={e => setV(e.target.value)} placeholder={placeholder} sx={{ width: 65 }} />
      <Button size="small" variant="outlined" onClick={() => onSave(parseInt(v) || 0)} sx={{ minWidth: 28, px: '2px' }}>✓</Button>
    </Stack>
  )
}
