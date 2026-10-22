package webui

import (
	"math"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/mumax/3/engine"
	"github.com/mumax/3/log"
)

type TablePlotState struct {
	ws               *WebSocketManager
	previewColumns   []string
	previewUnits     []string
	previewData      map[string][]float64
	AutoSaveInterval float64     `msgpack:"autoSaveInterval"`
	Columns          []string    `msgpack:"columns"`
	XColumn          string      `msgpack:"xColumn"`
	YColumn          string      `msgpack:"yColumn"`
	XColumnUnit      string      `msgpack:"xColumnUnit"`
	YColumnUnit      string      `msgpack:"yColumnUnit"`
	Data             [][]float64 `msgpack:"data"`
	XMin             float64     `msgpack:"xmin"`
	XMax             float64     `msgpack:"xmax"`
	YMin             float64     `msgpack:"ymin"`
	YMax             float64     `msgpack:"ymax"`
	MaxPoints        int         `msgpack:"maxPoints"`
	Step             int         `msgpack:"step"`
	CorePos          [][]float64 `msgpack:"corePos"`
	CoreEnabled      bool        `msgpack:"coreEnabled"`
	CoreMode         string      `msgpack:"coreMode"`
	CoreStatus       string      `msgpack:"coreStatus"`
}

func initTablePlotAPI(e *echo.Group, ws *WebSocketManager) *TablePlotState {
	t := TablePlotState{
		ws:               ws,
		AutoSaveInterval: engine.TableAutoSavePeriod(),
		XColumn:          "t",
		YColumn:          "mx",
		MaxPoints:        10000,
		Step:             1,
	}
	e.POST("/api/tableplot/autosave-interval", t.postTablePlotAutoSaveInterval)
	e.POST("/api/tableplot/xcolumn", t.postTablePlotXColumn)
	e.POST("/api/tableplot/ycolumn", t.postTablePlotYColumn)
	e.POST("/api/tableplot/maxpoints", t.postTablePlotMaxPoints)
	e.POST("/api/tableplot/step", t.postTablePlotStep)
	t.Update()
	return &t
}

func (t *TablePlotState) Update() {
	t.CoreMode, t.CoreStatus = engine.CoreTrackingStatus()
	t.CoreEnabled = t.CoreMode != ""
	t.AutoSaveInterval = engine.TableAutoSavePeriod()
	t.previewColumns, t.previewUnits, t.previewData = engine.TablePreviewSnapshot(
		[]string{t.XColumn, t.YColumn, "t", "ext_coreposx", "ext_coreposy", "ext_corevalid", "ext_skyrmionposx", "ext_skyrmionposy", "ext_skyrmionvalid"}, t.MaxPoints, t.Step)
	t.Columns = t.previewColumns
	t.XColumnUnit = t.GetUnit(t.XColumn)
	t.YColumnUnit = t.GetUnit(t.YColumn)
	t.GetTablePlotData()
	t.GetMinMaxXY()
	t.GetCorePosData()
}

func (t *TablePlotState) GetTablePlotData() {
	xData := t.GetColumnData(t.XColumn)
	yData := t.GetColumnData(t.YColumn)
	n := min(len(xData), len(yData), max(t.MaxPoints, 0))
	xData = xData[len(xData)-n:]
	yData = yData[len(yData)-n:]
	data := make([][]float64, n)
	for i := 0; i < len(xData); i++ {
		data[i] = []float64{xData[i], yData[i]}
	}
	t.Data = data
}

func (t *TablePlotState) GetMinMaxXY() {
	t.XMin, t.XMax, t.YMin, t.YMax = 0, 0, 0, 0
	initialized := false
	for _, row := range t.Data {
		if len(row) < 2 || !finiteCoreValue(row[0]) || !finiteCoreValue(row[1]) {
			continue
		}
		if !initialized {
			t.XMin, t.XMax, t.YMin, t.YMax = row[0], row[0], row[1], row[1]
			initialized = true
			continue
		}
		t.XMin = math.Min(t.XMin, row[0])
		t.XMax = math.Max(t.XMax, row[0])
		t.YMin = math.Min(t.YMin, row[1])
		t.YMax = math.Max(t.YMax, row[1])
	}
}

func (t *TablePlotState) GetCorePosData() {
	prefix, validColumn := "ext_corepos", "ext_corevalid"
	if t.CoreMode == "skyrmion" || (t.CoreMode == "" && t.ColumnExists("ext_skyrmionposx")) {
		prefix = "ext_skyrmionpos"
		validColumn = "ext_skyrmionvalid"
		t.CoreMode = "skyrmion"
	}
	if !t.ColumnExists(prefix+"x") || !t.ColumnExists(prefix+"y") || !t.ColumnExists("t") {
		t.CorePos = nil
		return
	}
	times, xs, ys := t.GetColumnData("t"), t.GetColumnData(prefix+"x"), t.GetColumnData(prefix+"y")
	valid := t.GetColumnData(validColumn)
	hasValidity := t.ColumnExists(validColumn)
	n := min(len(times), len(xs), len(ys))
	start := max(0, n-max(t.MaxPoints, 0))
	data := make([][]float64, 0, n-start)
	for i := start; i < n; i++ {
		ok := finiteCoreValue(times[i]) && finiteCoreValue(xs[i]) && finiteCoreValue(ys[i])
		if hasValidity {
			ok = ok && i < len(valid) && valid[i] == 1
		}
		if ok {
			data = append(data, []float64{times[i], xs[i], ys[i], 1})
		} else {
			tm := times[i]
			if !finiteCoreValue(tm) {
				tm = 0
			}
			data = append(data, []float64{tm, 0, 0, 0})
		}
	}
	t.CorePos = data
	if len(data) > 0 && data[len(data)-1][3] == 0 && t.CoreStatus == "" {
		t.CoreStatus = "core not localized"
	}
}

func finiteCoreValue(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (t *TablePlotState) GetColumnData(column string) []float64 { return t.previewData[column] }

func (t *TablePlotState) GetUnit(name string) string {
	columns, units := t.previewColumns, t.previewUnits
	if columns == nil {
		columns, units, _ = engine.TablePreviewSnapshot(nil, 0, 1)
	}
	for index, column := range columns {
		if column == name {
			return units[index]
		}
	}
	return ""
}

func (t *TablePlotState) ColumnExists(name string) bool {
	columns := t.previewColumns
	if columns == nil {
		columns, _, _ = engine.TablePreviewSnapshot(nil, 0, 1)
	}
	for _, column := range columns {
		if column == name {
			return true
		}
	}
	return false
}

func (t *TablePlotState) GetTableNames() []string {
	columns, _, _ := engine.TablePreviewSnapshot(nil, 0, 1)
	if columns == nil {
		return []string{}
	}
	return columns
}

func (t *TablePlotState) postTablePlotAutoSaveInterval(c echo.Context) error {
	t.ws.stateMu.Lock()
	defer t.ws.stateMu.Unlock()
	type Request struct {
		AutoSaveInterval string `msgpack:"autoSaveInterval"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	engine.InjectAndWait(func() { engine.EvalTryRecover("TableAutoSave(" + req.AutoSaveInterval + ")") })
	t.ws.broadcastEngineStateWithoutPreviewLocked()
	return c.JSON(http.StatusOK, nil)
}

func (t *TablePlotState) postTablePlotXColumn(c echo.Context) error {
	t.ws.stateMu.Lock()
	defer t.ws.stateMu.Unlock()
	type Request struct {
		XColumn string `msgpack:"XColumn"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if !t.ColumnExists(req.XColumn) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Table column not found"})
	}
	t.XColumn = req.XColumn
	t.ws.broadcastEngineStateWithoutPreviewLocked()
	return c.JSON(http.StatusOK, nil)
}

func (t *TablePlotState) postTablePlotYColumn(c echo.Context) error {
	t.ws.stateMu.Lock()
	defer t.ws.stateMu.Unlock()
	type Request struct {
		YColumn string `msgpack:"YColumn"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if !t.ColumnExists(req.YColumn) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Table column not found"})
	}
	t.YColumn = req.YColumn
	t.ws.broadcastEngineStateWithoutPreviewLocked()
	return c.JSON(http.StatusOK, nil)
}

func (t *TablePlotState) postTablePlotMaxPoints(c echo.Context) error {
	t.ws.stateMu.Lock()
	defer t.ws.stateMu.Unlock()
	type Request struct {
		MaxPoints int `msgpack:"maxPoints"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if req.MaxPoints < 1 || req.MaxPoints > 100000 {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "MaxPoints must be between 1 and 100000"})
	}
	t.MaxPoints = req.MaxPoints
	t.ws.broadcastEngineStateWithoutPreviewLocked()
	return c.JSON(http.StatusOK, nil)
}

func (t *TablePlotState) postTablePlotStep(c echo.Context) error {
	t.ws.stateMu.Lock()
	defer t.ws.stateMu.Unlock()
	type Request struct {
		Step int `msgpack:"step"`
	}
	req := new(Request)
	if err := c.Bind(req); err != nil {
		log.Log.Err("%v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request payload"})
	}
	if req.Step < 1 {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Step must be at least 1"})
	}
	t.Step = req.Step
	t.ws.broadcastEngineStateWithoutPreviewLocked()
	return c.JSON(http.StatusOK, nil)
}
