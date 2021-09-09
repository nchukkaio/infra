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
    Cred,
    CredFromJSON,
    CredToJSON,
} from '../models';

/**
 * 
 */
export class CredsApi extends runtime.BaseAPI {

    /**
     * Create credentials to access a destination
     */
    async createCredRaw(initOverrides?: RequestInit): Promise<runtime.ApiResponse<Cred>> {
        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        if (this.configuration && this.configuration.accessToken) {
            const token = this.configuration.accessToken;
            const tokenString = await token("bearerAuth", []);

            if (tokenString) {
                headerParameters["Authorization"] = `Bearer ${tokenString}`;
            }
        }
        const response = await this.request({
            path: `/creds`,
            method: 'POST',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => CredFromJSON(jsonValue));
    }

    /**
     * Create credentials to access a destination
     */
    async createCred(initOverrides?: RequestInit): Promise<Cred> {
        const response = await this.createCredRaw(initOverrides);
        return await response.value();
    }

}