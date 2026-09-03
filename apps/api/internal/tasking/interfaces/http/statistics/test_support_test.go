package statisticshttp

import statisticsapp "ideaboard/api/internal/tasking/application/statistics"

type Service = statisticsapp.Service
type MonitorExecutionPressurePoint = statisticsapp.MonitorExecutionPressurePoint
type MonitorGoldenSignals = statisticsapp.MonitorGoldenSignals

func NewService(store statisticsapp.Store) *Service { return statisticsapp.NewService(store) }

var monitorExecutionPressureBuckets = make([]struct{}, 11)
