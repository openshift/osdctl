package utils

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type ValidateServiceFilePathCallback func(filePath string) string

type ServicesRegistry struct {
	appInterfaceClone   *AppInterfaceClone
	serviceIdToFilePath map[string]string
}

func NewServicesRegistry(appInterfaceClone *AppInterfaceClone, validateServiceFilePathCallback ValidateServiceFilePathCallback, servicesDirsRelPaths ...string) (*ServicesRegistry, error) {
	rootDirPath := appInterfaceClone.GetPath()
	serviceIdToFilePath := make(map[string]string)

	for _, servicesDirRelPath := range servicesDirsRelPaths {
		servicesDirPath := filepath.Join(rootDirPath, servicesDirRelPath)
		fileEntries, err := os.ReadDir(servicesDirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory '%s': %v", servicesDirPath, err)
		}

		for _, fileEntry := range fileEntries {
			fileName := fileEntry.Name()
			filePath := filepath.Join(servicesDirPath, fileName)
			serviceFilePath := validateServiceFilePathCallback(filePath)

			if serviceFilePath != "" {
				serviceId := strings.TrimSuffix(fileName, filepath.Ext(fileName))
				serviceIdToFilePath[serviceId] = serviceFilePath
			}
		}
	}

	return &ServicesRegistry{
		appInterfaceClone:   appInterfaceClone,
		serviceIdToFilePath: serviceIdToFilePath}, nil
}

func (r *ServicesRegistry) GetServicesIds() []string {
	return slices.Sorted(maps.Keys(r.serviceIdToFilePath))
}

func (r *ServicesRegistry) GetService(serviceId string) (*Service, error) {
	if serviceFilePath, ok := r.serviceIdToFilePath[serviceId]; ok {
		return ReadServiceFromFile(r.appInterfaceClone, serviceFilePath)
	}

	return nil, fmt.Errorf("no SaaS file for the following service: %s", serviceId)
}
