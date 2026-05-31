import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  TextField, Select, MenuItem, FormControl, IconButton, Chip,
} from '@mui/material'
import { Add, Delete } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { CategoryMapping, CSCartCategory } from '../types'

export default function Mapping() {
  const [otCat, setOtCat] = useState('')
  const [csCat, setCsCat] = useState('')
  const [csCatName, setCsCatName] = useState('')
  const [notes, setNotes] = useState('')
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

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: '4px' }}>Mapping категорий</Typography>
      <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 2 }}>
        OT Commerce → CS-Cart. Только замапленные категории синхронизируются на wabrum.com.
      </Typography>

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
                    <TableCell>Цена CNY (min-max)</TableCell>
                    <TableCell>Мин. продаж</TableCell>
                    <TableCell></TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {mappings.map(m => (
                    <TableRow key={m.OTCategoryID} hover>
                      <TableCell>
                        <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{m.OTCategoryID}</Typography>
                        {m.Notes && <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{m.Notes}</Typography>}
                      </TableCell>
                      <TableCell>
                        <Chip label={m.CSCategoryName} color="success" size="small" />
                        <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: '4px' }}>#{m.CSCategoryID}</Typography>
                      </TableCell>
                      <TableCell>
                        <InlineInput value={m.WeightG} placeholder="г"
                          onSave={v => api.post('/mapping/set-weight', { ot_category_id: m.OTCategoryID, weight_g: v }).then(() => qc.invalidateQueries({ queryKey: ['mapping'] }))} />
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
                    {unmappedOT.map((c: any) => (
                      <MenuItem key={c.ID} value={c.ID} sx={{ pl: c.IsChild ? 3.5 : 1.5 }}>
                        {c.IsChild ? <Typography component="span" sx={{ color: 'text.disabled', mr: 0.5 }}>└</Typography> : null}
                        <Typography component="span" sx={{ fontSize: 13, fontWeight: c.IsChild ? 400 : 600 }}>
                          {c.Name}
                        </Typography>
                        {c.ItemCount > 0 && (
                          <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>
                            ({c.ItemCount >= 1000 ? Math.round(c.ItemCount/1000)+'K' : c.ItemCount} тов.)
                          </Typography>
                        )}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
                <Typography sx={{ fontSize: 11, color: 'text.secondary', mt: '4px' }}>Незамапленных: {unmappedOT.length}</Typography>
              </Box>

              <Box sx={{ alignSelf: 'center', pt: 2 }}>
                <Typography sx={{ fontSize: 22, color: 'text.disabled' }}>→</Typography>
              </Box>

              <Box sx={{ flex: 1 }}>
                <Typography sx={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', color: 'text.secondary', mb: '4px', letterSpacing: '0.05em' }}>
                  CS-Cart wabrum.com — куда загружаем
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
                      <MenuItem key={c.CategoryID} value={String(c.CategoryID)}>
                        {c.ParentID > 0 ? '  └ ' : ''}{c.Name} (#{c.CategoryID})
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
    </Box>
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
