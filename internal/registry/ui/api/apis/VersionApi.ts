/* tslint:disable */
/* eslint-disable */
/**
 * Infra API
 * Infra REST API
 *
 * The version of the OpenAPI document: 0.1.0
 * 
 *
 * NOTE: This class is auto generated by OpenAPI Generator (https://openapi-generator.tech).
 * https://openapi-generator.tech
 * Do not edit the class manually.
 */


import * as runtime from '../runtime';
import {
    Version,
    VersionFromJSON,
    VersionToJSON,
} from '../models';

/**
 * 
 */
export class VersionApi extends runtime.BaseAPI {

    /**
     * Get version information
     */
    async versionRaw(initOverrides?: RequestInit): Promise<runtime.ApiResponse<Version>> {
        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        const response = await this.request({
            path: `/version`,
            method: 'GET',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => VersionFromJSON(jsonValue));
    }

    /**
     * Get version information
     */
    async version(initOverrides?: RequestInit): Promise<Version> {
        const response = await this.versionRaw(initOverrides);
        return await response.value();
    }

}