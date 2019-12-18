/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

// Code generated from ZitiQl.g4 by ANTLR 4.7.1. DO NOT EDIT.

package zitiql // ZitiQl
import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/antlr/antlr4/runtime/Go/antlr"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = reflect.Copy
var _ = strconv.Itoa

var parserATN = []uint16{
	3, 24715, 42794, 33075, 47597, 16764, 15335, 30598, 22884, 3, 23, 439,
	4, 2, 9, 2, 4, 3, 9, 3, 4, 4, 9, 4, 4, 5, 9, 5, 4, 6, 9, 6, 4, 7, 9, 7,
	3, 2, 3, 2, 7, 2, 17, 10, 2, 12, 2, 14, 2, 20, 11, 2, 3, 2, 3, 2, 7, 2,
	24, 10, 2, 12, 2, 14, 2, 27, 11, 2, 3, 2, 3, 2, 7, 2, 31, 10, 2, 12, 2,
	14, 2, 34, 11, 2, 3, 2, 7, 2, 37, 10, 2, 12, 2, 14, 2, 40, 11, 2, 3, 2,
	7, 2, 43, 10, 2, 12, 2, 14, 2, 46, 11, 2, 3, 2, 3, 2, 3, 3, 3, 3, 7, 3,
	52, 10, 3, 12, 3, 14, 3, 55, 11, 3, 3, 3, 3, 3, 7, 3, 59, 10, 3, 12, 3,
	14, 3, 62, 11, 3, 3, 3, 3, 3, 7, 3, 66, 10, 3, 12, 3, 14, 3, 69, 11, 3,
	3, 3, 7, 3, 72, 10, 3, 12, 3, 14, 3, 75, 11, 3, 3, 3, 7, 3, 78, 10, 3,
	12, 3, 14, 3, 81, 11, 3, 3, 3, 3, 3, 3, 4, 3, 4, 7, 4, 87, 10, 4, 12, 4,
	14, 4, 90, 11, 4, 3, 4, 3, 4, 7, 4, 94, 10, 4, 12, 4, 14, 4, 97, 11, 4,
	3, 4, 3, 4, 7, 4, 101, 10, 4, 12, 4, 14, 4, 104, 11, 4, 3, 4, 7, 4, 107,
	10, 4, 12, 4, 14, 4, 110, 11, 4, 3, 4, 7, 4, 113, 10, 4, 12, 4, 14, 4,
	116, 11, 4, 3, 4, 3, 4, 3, 5, 7, 5, 121, 10, 5, 12, 5, 14, 5, 124, 11,
	5, 3, 5, 7, 5, 127, 10, 5, 12, 5, 14, 5, 130, 11, 5, 3, 5, 7, 5, 133, 10,
	5, 12, 5, 14, 5, 136, 11, 5, 3, 5, 3, 5, 3, 6, 3, 6, 3, 6, 3, 6, 7, 6,
	144, 10, 6, 12, 6, 14, 6, 147, 11, 6, 3, 6, 3, 6, 7, 6, 151, 10, 6, 12,
	6, 14, 6, 154, 11, 6, 3, 6, 3, 6, 5, 6, 158, 10, 6, 3, 6, 3, 6, 6, 6, 162,
	10, 6, 13, 6, 14, 6, 163, 3, 6, 3, 6, 6, 6, 168, 10, 6, 13, 6, 14, 6, 169,
	3, 6, 6, 6, 173, 10, 6, 13, 6, 14, 6, 174, 3, 6, 3, 6, 6, 6, 179, 10, 6,
	13, 6, 14, 6, 180, 3, 6, 3, 6, 6, 6, 185, 10, 6, 13, 6, 14, 6, 186, 3,
	6, 6, 6, 190, 10, 6, 13, 6, 14, 6, 191, 7, 6, 194, 10, 6, 12, 6, 14, 6,
	197, 11, 6, 3, 7, 3, 7, 6, 7, 201, 10, 7, 13, 7, 14, 7, 202, 3, 7, 3, 7,
	6, 7, 207, 10, 7, 13, 7, 14, 7, 208, 3, 7, 3, 7, 3, 7, 6, 7, 214, 10, 7,
	13, 7, 14, 7, 215, 3, 7, 3, 7, 6, 7, 220, 10, 7, 13, 7, 14, 7, 221, 3,
	7, 3, 7, 3, 7, 6, 7, 227, 10, 7, 13, 7, 14, 7, 228, 3, 7, 3, 7, 6, 7, 233,
	10, 7, 13, 7, 14, 7, 234, 3, 7, 3, 7, 3, 7, 6, 7, 240, 10, 7, 13, 7, 14,
	7, 241, 3, 7, 3, 7, 6, 7, 246, 10, 7, 13, 7, 14, 7, 247, 3, 7, 3, 7, 6,
	7, 252, 10, 7, 13, 7, 14, 7, 253, 3, 7, 3, 7, 6, 7, 258, 10, 7, 13, 7,
	14, 7, 259, 3, 7, 3, 7, 3, 7, 6, 7, 265, 10, 7, 13, 7, 14, 7, 266, 3, 7,
	3, 7, 6, 7, 271, 10, 7, 13, 7, 14, 7, 272, 3, 7, 3, 7, 6, 7, 277, 10, 7,
	13, 7, 14, 7, 278, 3, 7, 3, 7, 6, 7, 283, 10, 7, 13, 7, 14, 7, 284, 3,
	7, 3, 7, 3, 7, 7, 7, 290, 10, 7, 12, 7, 14, 7, 293, 11, 7, 3, 7, 3, 7,
	7, 7, 297, 10, 7, 12, 7, 14, 7, 300, 11, 7, 3, 7, 3, 7, 3, 7, 7, 7, 305,
	10, 7, 12, 7, 14, 7, 308, 11, 7, 3, 7, 3, 7, 7, 7, 312, 10, 7, 12, 7, 14,
	7, 315, 11, 7, 3, 7, 3, 7, 3, 7, 7, 7, 320, 10, 7, 12, 7, 14, 7, 323, 11,
	7, 3, 7, 3, 7, 7, 7, 327, 10, 7, 12, 7, 14, 7, 330, 11, 7, 3, 7, 3, 7,
	3, 7, 7, 7, 335, 10, 7, 12, 7, 14, 7, 338, 11, 7, 3, 7, 3, 7, 7, 7, 342,
	10, 7, 12, 7, 14, 7, 345, 11, 7, 3, 7, 3, 7, 3, 7, 7, 7, 350, 10, 7, 12,
	7, 14, 7, 353, 11, 7, 3, 7, 3, 7, 7, 7, 357, 10, 7, 12, 7, 14, 7, 360,
	11, 7, 3, 7, 3, 7, 3, 7, 7, 7, 365, 10, 7, 12, 7, 14, 7, 368, 11, 7, 3,
	7, 3, 7, 7, 7, 372, 10, 7, 12, 7, 14, 7, 375, 11, 7, 3, 7, 3, 7, 3, 7,
	7, 7, 380, 10, 7, 12, 7, 14, 7, 383, 11, 7, 3, 7, 3, 7, 7, 7, 387, 10,
	7, 12, 7, 14, 7, 390, 11, 7, 3, 7, 3, 7, 3, 7, 7, 7, 395, 10, 7, 12, 7,
	14, 7, 398, 11, 7, 3, 7, 3, 7, 7, 7, 402, 10, 7, 12, 7, 14, 7, 405, 11,
	7, 3, 7, 3, 7, 3, 7, 7, 7, 410, 10, 7, 12, 7, 14, 7, 413, 11, 7, 3, 7,
	3, 7, 7, 7, 417, 10, 7, 12, 7, 14, 7, 420, 11, 7, 3, 7, 3, 7, 3, 7, 7,
	7, 425, 10, 7, 12, 7, 14, 7, 428, 11, 7, 3, 7, 3, 7, 6, 7, 432, 10, 7,
	13, 7, 14, 7, 433, 3, 7, 5, 7, 437, 10, 7, 3, 7, 2, 3, 10, 8, 2, 4, 6,
	8, 10, 12, 2, 3, 3, 2, 19, 20, 2, 509, 2, 14, 3, 2, 2, 2, 4, 49, 3, 2,
	2, 2, 6, 84, 3, 2, 2, 2, 8, 122, 3, 2, 2, 2, 10, 157, 3, 2, 2, 2, 12, 436,
	3, 2, 2, 2, 14, 18, 7, 7, 2, 2, 15, 17, 7, 4, 2, 2, 16, 15, 3, 2, 2, 2,
	17, 20, 3, 2, 2, 2, 18, 16, 3, 2, 2, 2, 18, 19, 3, 2, 2, 2, 19, 21, 3,
	2, 2, 2, 20, 18, 3, 2, 2, 2, 21, 38, 7, 19, 2, 2, 22, 24, 7, 4, 2, 2, 23,
	22, 3, 2, 2, 2, 24, 27, 3, 2, 2, 2, 25, 23, 3, 2, 2, 2, 25, 26, 3, 2, 2,
	2, 26, 28, 3, 2, 2, 2, 27, 25, 3, 2, 2, 2, 28, 32, 7, 3, 2, 2, 29, 31,
	7, 4, 2, 2, 30, 29, 3, 2, 2, 2, 31, 34, 3, 2, 2, 2, 32, 30, 3, 2, 2, 2,
	32, 33, 3, 2, 2, 2, 33, 35, 3, 2, 2, 2, 34, 32, 3, 2, 2, 2, 35, 37, 7,
	19, 2, 2, 36, 25, 3, 2, 2, 2, 37, 40, 3, 2, 2, 2, 38, 36, 3, 2, 2, 2, 38,
	39, 3, 2, 2, 2, 39, 44, 3, 2, 2, 2, 40, 38, 3, 2, 2, 2, 41, 43, 7, 4, 2,
	2, 42, 41, 3, 2, 2, 2, 43, 46, 3, 2, 2, 2, 44, 42, 3, 2, 2, 2, 44, 45,
	3, 2, 2, 2, 45, 47, 3, 2, 2, 2, 46, 44, 3, 2, 2, 2, 47, 48, 7, 8, 2, 2,
	48, 3, 3, 2, 2, 2, 49, 53, 7, 7, 2, 2, 50, 52, 7, 4, 2, 2, 51, 50, 3, 2,
	2, 2, 52, 55, 3, 2, 2, 2, 53, 51, 3, 2, 2, 2, 53, 54, 3, 2, 2, 2, 54, 56,
	3, 2, 2, 2, 55, 53, 3, 2, 2, 2, 56, 73, 7, 20, 2, 2, 57, 59, 7, 4, 2, 2,
	58, 57, 3, 2, 2, 2, 59, 62, 3, 2, 2, 2, 60, 58, 3, 2, 2, 2, 60, 61, 3,
	2, 2, 2, 61, 63, 3, 2, 2, 2, 62, 60, 3, 2, 2, 2, 63, 67, 7, 3, 2, 2, 64,
	66, 7, 4, 2, 2, 65, 64, 3, 2, 2, 2, 66, 69, 3, 2, 2, 2, 67, 65, 3, 2, 2,
	2, 67, 68, 3, 2, 2, 2, 68, 70, 3, 2, 2, 2, 69, 67, 3, 2, 2, 2, 70, 72,
	7, 20, 2, 2, 71, 60, 3, 2, 2, 2, 72, 75, 3, 2, 2, 2, 73, 71, 3, 2, 2, 2,
	73, 74, 3, 2, 2, 2, 74, 79, 3, 2, 2, 2, 75, 73, 3, 2, 2, 2, 76, 78, 7,
	4, 2, 2, 77, 76, 3, 2, 2, 2, 78, 81, 3, 2, 2, 2, 79, 77, 3, 2, 2, 2, 79,
	80, 3, 2, 2, 2, 80, 82, 3, 2, 2, 2, 81, 79, 3, 2, 2, 2, 82, 83, 7, 8, 2,
	2, 83, 5, 3, 2, 2, 2, 84, 88, 7, 7, 2, 2, 85, 87, 7, 4, 2, 2, 86, 85, 3,
	2, 2, 2, 87, 90, 3, 2, 2, 2, 88, 86, 3, 2, 2, 2, 88, 89, 3, 2, 2, 2, 89,
	91, 3, 2, 2, 2, 90, 88, 3, 2, 2, 2, 91, 108, 7, 18, 2, 2, 92, 94, 7, 4,
	2, 2, 93, 92, 3, 2, 2, 2, 94, 97, 3, 2, 2, 2, 95, 93, 3, 2, 2, 2, 95, 96,
	3, 2, 2, 2, 96, 98, 3, 2, 2, 2, 97, 95, 3, 2, 2, 2, 98, 102, 7, 3, 2, 2,
	99, 101, 7, 4, 2, 2, 100, 99, 3, 2, 2, 2, 101, 104, 3, 2, 2, 2, 102, 100,
	3, 2, 2, 2, 102, 103, 3, 2, 2, 2, 103, 105, 3, 2, 2, 2, 104, 102, 3, 2,
	2, 2, 105, 107, 7, 18, 2, 2, 106, 95, 3, 2, 2, 2, 107, 110, 3, 2, 2, 2,
	108, 106, 3, 2, 2, 2, 108, 109, 3, 2, 2, 2, 109, 114, 3, 2, 2, 2, 110,
	108, 3, 2, 2, 2, 111, 113, 7, 4, 2, 2, 112, 111, 3, 2, 2, 2, 113, 116,
	3, 2, 2, 2, 114, 112, 3, 2, 2, 2, 114, 115, 3, 2, 2, 2, 115, 117, 3, 2,
	2, 2, 116, 114, 3, 2, 2, 2, 117, 118, 7, 8, 2, 2, 118, 7, 3, 2, 2, 2, 119,
	121, 7, 4, 2, 2, 120, 119, 3, 2, 2, 2, 121, 124, 3, 2, 2, 2, 122, 120,
	3, 2, 2, 2, 122, 123, 3, 2, 2, 2, 123, 128, 3, 2, 2, 2, 124, 122, 3, 2,
	2, 2, 125, 127, 5, 10, 6, 2, 126, 125, 3, 2, 2, 2, 127, 130, 3, 2, 2, 2,
	128, 126, 3, 2, 2, 2, 128, 129, 3, 2, 2, 2, 129, 134, 3, 2, 2, 2, 130,
	128, 3, 2, 2, 2, 131, 133, 7, 4, 2, 2, 132, 131, 3, 2, 2, 2, 133, 136,
	3, 2, 2, 2, 134, 132, 3, 2, 2, 2, 134, 135, 3, 2, 2, 2, 135, 137, 3, 2,
	2, 2, 136, 134, 3, 2, 2, 2, 137, 138, 7, 2, 2, 3, 138, 9, 3, 2, 2, 2, 139,
	140, 8, 6, 1, 2, 140, 158, 5, 12, 7, 2, 141, 145, 7, 5, 2, 2, 142, 144,
	7, 4, 2, 2, 143, 142, 3, 2, 2, 2, 144, 147, 3, 2, 2, 2, 145, 143, 3, 2,
	2, 2, 145, 146, 3, 2, 2, 2, 146, 148, 3, 2, 2, 2, 147, 145, 3, 2, 2, 2,
	148, 152, 5, 10, 6, 2, 149, 151, 7, 4, 2, 2, 150, 149, 3, 2, 2, 2, 151,
	154, 3, 2, 2, 2, 152, 150, 3, 2, 2, 2, 152, 153, 3, 2, 2, 2, 153, 155,
	3, 2, 2, 2, 154, 152, 3, 2, 2, 2, 155, 156, 7, 6, 2, 2, 156, 158, 3, 2,
	2, 2, 157, 139, 3, 2, 2, 2, 157, 141, 3, 2, 2, 2, 158, 195, 3, 2, 2, 2,
	159, 172, 12, 4, 2, 2, 160, 162, 7, 4, 2, 2, 161, 160, 3, 2, 2, 2, 162,
	163, 3, 2, 2, 2, 163, 161, 3, 2, 2, 2, 163, 164, 3, 2, 2, 2, 164, 165,
	3, 2, 2, 2, 165, 167, 7, 9, 2, 2, 166, 168, 7, 4, 2, 2, 167, 166, 3, 2,
	2, 2, 168, 169, 3, 2, 2, 2, 169, 167, 3, 2, 2, 2, 169, 170, 3, 2, 2, 2,
	170, 171, 3, 2, 2, 2, 171, 173, 5, 10, 6, 2, 172, 161, 3, 2, 2, 2, 173,
	174, 3, 2, 2, 2, 174, 172, 3, 2, 2, 2, 174, 175, 3, 2, 2, 2, 175, 194,
	3, 2, 2, 2, 176, 189, 12, 3, 2, 2, 177, 179, 7, 4, 2, 2, 178, 177, 3, 2,
	2, 2, 179, 180, 3, 2, 2, 2, 180, 178, 3, 2, 2, 2, 180, 181, 3, 2, 2, 2,
	181, 182, 3, 2, 2, 2, 182, 184, 7, 10, 2, 2, 183, 185, 7, 4, 2, 2, 184,
	183, 3, 2, 2, 2, 185, 186, 3, 2, 2, 2, 186, 184, 3, 2, 2, 2, 186, 187,
	3, 2, 2, 2, 187, 188, 3, 2, 2, 2, 188, 190, 5, 10, 6, 2, 189, 178, 3, 2,
	2, 2, 190, 191, 3, 2, 2, 2, 191, 189, 3, 2, 2, 2, 191, 192, 3, 2, 2, 2,
	192, 194, 3, 2, 2, 2, 193, 159, 3, 2, 2, 2, 193, 176, 3, 2, 2, 2, 194,
	197, 3, 2, 2, 2, 195, 193, 3, 2, 2, 2, 195, 196, 3, 2, 2, 2, 196, 11, 3,
	2, 2, 2, 197, 195, 3, 2, 2, 2, 198, 200, 7, 22, 2, 2, 199, 201, 7, 4, 2,
	2, 200, 199, 3, 2, 2, 2, 201, 202, 3, 2, 2, 2, 202, 200, 3, 2, 2, 2, 202,
	203, 3, 2, 2, 2, 203, 204, 3, 2, 2, 2, 204, 206, 7, 15, 2, 2, 205, 207,
	7, 4, 2, 2, 206, 205, 3, 2, 2, 2, 207, 208, 3, 2, 2, 2, 208, 206, 3, 2,
	2, 2, 208, 209, 3, 2, 2, 2, 209, 210, 3, 2, 2, 2, 210, 437, 5, 2, 2, 2,
	211, 213, 7, 22, 2, 2, 212, 214, 7, 4, 2, 2, 213, 212, 3, 2, 2, 2, 214,
	215, 3, 2, 2, 2, 215, 213, 3, 2, 2, 2, 215, 216, 3, 2, 2, 2, 216, 217,
	3, 2, 2, 2, 217, 219, 7, 15, 2, 2, 218, 220, 7, 4, 2, 2, 219, 218, 3, 2,
	2, 2, 220, 221, 3, 2, 2, 2, 221, 219, 3, 2, 2, 2, 221, 222, 3, 2, 2, 2,
	222, 223, 3, 2, 2, 2, 223, 437, 5, 4, 3, 2, 224, 226, 7, 22, 2, 2, 225,
	227, 7, 4, 2, 2, 226, 225, 3, 2, 2, 2, 227, 228, 3, 2, 2, 2, 228, 226,
	3, 2, 2, 2, 228, 229, 3, 2, 2, 2, 229, 230, 3, 2, 2, 2, 230, 232, 7, 15,
	2, 2, 231, 233, 7, 4, 2, 2, 232, 231, 3, 2, 2, 2, 233, 234, 3, 2, 2, 2,
	234, 232, 3, 2, 2, 2, 234, 235, 3, 2, 2, 2, 235, 236, 3, 2, 2, 2, 236,
	437, 5, 6, 4, 2, 237, 239, 7, 22, 2, 2, 238, 240, 7, 4, 2, 2, 239, 238,
	3, 2, 2, 2, 240, 241, 3, 2, 2, 2, 241, 239, 3, 2, 2, 2, 241, 242, 3, 2,
	2, 2, 242, 243, 3, 2, 2, 2, 243, 245, 7, 16, 2, 2, 244, 246, 7, 4, 2, 2,
	245, 244, 3, 2, 2, 2, 246, 247, 3, 2, 2, 2, 247, 245, 3, 2, 2, 2, 247,
	248, 3, 2, 2, 2, 248, 249, 3, 2, 2, 2, 249, 251, 7, 20, 2, 2, 250, 252,
	7, 4, 2, 2, 251, 250, 3, 2, 2, 2, 252, 253, 3, 2, 2, 2, 253, 251, 3, 2,
	2, 2, 253, 254, 3, 2, 2, 2, 254, 255, 3, 2, 2, 2, 255, 257, 7, 9, 2, 2,
	256, 258, 7, 4, 2, 2, 257, 256, 3, 2, 2, 2, 258, 259, 3, 2, 2, 2, 259,
	257, 3, 2, 2, 2, 259, 260, 3, 2, 2, 2, 260, 261, 3, 2, 2, 2, 261, 437,
	7, 20, 2, 2, 262, 264, 7, 22, 2, 2, 263, 265, 7, 4, 2, 2, 264, 263, 3,
	2, 2, 2, 265, 266, 3, 2, 2, 2, 266, 264, 3, 2, 2, 2, 266, 267, 3, 2, 2,
	2, 267, 268, 3, 2, 2, 2, 268, 270, 7, 16, 2, 2, 269, 271, 7, 4, 2, 2, 270,
	269, 3, 2, 2, 2, 271, 272, 3, 2, 2, 2, 272, 270, 3, 2, 2, 2, 272, 273,
	3, 2, 2, 2, 273, 274, 3, 2, 2, 2, 274, 276, 7, 18, 2, 2, 275, 277, 7, 4,
	2, 2, 276, 275, 3, 2, 2, 2, 277, 278, 3, 2, 2, 2, 278, 276, 3, 2, 2, 2,
	278, 279, 3, 2, 2, 2, 279, 280, 3, 2, 2, 2, 280, 282, 7, 9, 2, 2, 281,
	283, 7, 4, 2, 2, 282, 281, 3, 2, 2, 2, 283, 284, 3, 2, 2, 2, 284, 282,
	3, 2, 2, 2, 284, 285, 3, 2, 2, 2, 285, 286, 3, 2, 2, 2, 286, 437, 7, 18,
	2, 2, 287, 291, 7, 22, 2, 2, 288, 290, 7, 4, 2, 2, 289, 288, 3, 2, 2, 2,
	290, 293, 3, 2, 2, 2, 291, 289, 3, 2, 2, 2, 291, 292, 3, 2, 2, 2, 292,
	294, 3, 2, 2, 2, 293, 291, 3, 2, 2, 2, 294, 298, 7, 11, 2, 2, 295, 297,
	7, 4, 2, 2, 296, 295, 3, 2, 2, 2, 297, 300, 3, 2, 2, 2, 298, 296, 3, 2,
	2, 2, 298, 299, 3, 2, 2, 2, 299, 301, 3, 2, 2, 2, 300, 298, 3, 2, 2, 2,
	301, 437, 7, 20, 2, 2, 302, 306, 7, 22, 2, 2, 303, 305, 7, 4, 2, 2, 304,
	303, 3, 2, 2, 2, 305, 308, 3, 2, 2, 2, 306, 304, 3, 2, 2, 2, 306, 307,
	3, 2, 2, 2, 307, 309, 3, 2, 2, 2, 308, 306, 3, 2, 2, 2, 309, 313, 7, 11,
	2, 2, 310, 312, 7, 4, 2, 2, 311, 310, 3, 2, 2, 2, 312, 315, 3, 2, 2, 2,
	313, 311, 3, 2, 2, 2, 313, 314, 3, 2, 2, 2, 314, 316, 3, 2, 2, 2, 315,
	313, 3, 2, 2, 2, 316, 437, 7, 18, 2, 2, 317, 321, 7, 22, 2, 2, 318, 320,
	7, 4, 2, 2, 319, 318, 3, 2, 2, 2, 320, 323, 3, 2, 2, 2, 321, 319, 3, 2,
	2, 2, 321, 322, 3, 2, 2, 2, 322, 324, 3, 2, 2, 2, 323, 321, 3, 2, 2, 2,
	324, 328, 7, 12, 2, 2, 325, 327, 7, 4, 2, 2, 326, 325, 3, 2, 2, 2, 327,
	330, 3, 2, 2, 2, 328, 326, 3, 2, 2, 2, 328, 329, 3, 2, 2, 2, 329, 331,
	3, 2, 2, 2, 330, 328, 3, 2, 2, 2, 331, 437, 7, 20, 2, 2, 332, 336, 7, 22,
	2, 2, 333, 335, 7, 4, 2, 2, 334, 333, 3, 2, 2, 2, 335, 338, 3, 2, 2, 2,
	336, 334, 3, 2, 2, 2, 336, 337, 3, 2, 2, 2, 337, 339, 3, 2, 2, 2, 338,
	336, 3, 2, 2, 2, 339, 343, 7, 12, 2, 2, 340, 342, 7, 4, 2, 2, 341, 340,
	3, 2, 2, 2, 342, 345, 3, 2, 2, 2, 343, 341, 3, 2, 2, 2, 343, 344, 3, 2,
	2, 2, 344, 346, 3, 2, 2, 2, 345, 343, 3, 2, 2, 2, 346, 437, 7, 18, 2, 2,
	347, 351, 7, 22, 2, 2, 348, 350, 7, 4, 2, 2, 349, 348, 3, 2, 2, 2, 350,
	353, 3, 2, 2, 2, 351, 349, 3, 2, 2, 2, 351, 352, 3, 2, 2, 2, 352, 354,
	3, 2, 2, 2, 353, 351, 3, 2, 2, 2, 354, 358, 7, 13, 2, 2, 355, 357, 7, 4,
	2, 2, 356, 355, 3, 2, 2, 2, 357, 360, 3, 2, 2, 2, 358, 356, 3, 2, 2, 2,
	358, 359, 3, 2, 2, 2, 359, 361, 3, 2, 2, 2, 360, 358, 3, 2, 2, 2, 361,
	437, 7, 19, 2, 2, 362, 366, 7, 22, 2, 2, 363, 365, 7, 4, 2, 2, 364, 363,
	3, 2, 2, 2, 365, 368, 3, 2, 2, 2, 366, 364, 3, 2, 2, 2, 366, 367, 3, 2,
	2, 2, 367, 369, 3, 2, 2, 2, 368, 366, 3, 2, 2, 2, 369, 373, 7, 13, 2, 2,
	370, 372, 7, 4, 2, 2, 371, 370, 3, 2, 2, 2, 372, 375, 3, 2, 2, 2, 373,
	371, 3, 2, 2, 2, 373, 374, 3, 2, 2, 2, 374, 376, 3, 2, 2, 2, 375, 373,
	3, 2, 2, 2, 376, 437, 7, 20, 2, 2, 377, 381, 7, 22, 2, 2, 378, 380, 7,
	4, 2, 2, 379, 378, 3, 2, 2, 2, 380, 383, 3, 2, 2, 2, 381, 379, 3, 2, 2,
	2, 381, 382, 3, 2, 2, 2, 382, 384, 3, 2, 2, 2, 383, 381, 3, 2, 2, 2, 384,
	388, 7, 13, 2, 2, 385, 387, 7, 4, 2, 2, 386, 385, 3, 2, 2, 2, 387, 390,
	3, 2, 2, 2, 388, 386, 3, 2, 2, 2, 388, 389, 3, 2, 2, 2, 389, 391, 3, 2,
	2, 2, 390, 388, 3, 2, 2, 2, 391, 437, 7, 18, 2, 2, 392, 396, 7, 22, 2,
	2, 393, 395, 7, 4, 2, 2, 394, 393, 3, 2, 2, 2, 395, 398, 3, 2, 2, 2, 396,
	394, 3, 2, 2, 2, 396, 397, 3, 2, 2, 2, 397, 399, 3, 2, 2, 2, 398, 396,
	3, 2, 2, 2, 399, 403, 7, 13, 2, 2, 400, 402, 7, 4, 2, 2, 401, 400, 3, 2,
	2, 2, 402, 405, 3, 2, 2, 2, 403, 401, 3, 2, 2, 2, 403, 404, 3, 2, 2, 2,
	404, 406, 3, 2, 2, 2, 405, 403, 3, 2, 2, 2, 406, 437, 7, 17, 2, 2, 407,
	411, 7, 22, 2, 2, 408, 410, 7, 4, 2, 2, 409, 408, 3, 2, 2, 2, 410, 413,
	3, 2, 2, 2, 411, 409, 3, 2, 2, 2, 411, 412, 3, 2, 2, 2, 412, 414, 3, 2,
	2, 2, 413, 411, 3, 2, 2, 2, 414, 418, 7, 13, 2, 2, 415, 417, 7, 4, 2, 2,
	416, 415, 3, 2, 2, 2, 417, 420, 3, 2, 2, 2, 418, 416, 3, 2, 2, 2, 418,
	419, 3, 2, 2, 2, 419, 421, 3, 2, 2, 2, 420, 418, 3, 2, 2, 2, 421, 437,
	7, 21, 2, 2, 422, 426, 7, 22, 2, 2, 423, 425, 7, 4, 2, 2, 424, 423, 3,
	2, 2, 2, 425, 428, 3, 2, 2, 2, 426, 424, 3, 2, 2, 2, 426, 427, 3, 2, 2,
	2, 427, 429, 3, 2, 2, 2, 428, 426, 3, 2, 2, 2, 429, 431, 7, 14, 2, 2, 430,
	432, 7, 4, 2, 2, 431, 430, 3, 2, 2, 2, 432, 433, 3, 2, 2, 2, 433, 431,
	3, 2, 2, 2, 433, 434, 3, 2, 2, 2, 434, 435, 3, 2, 2, 2, 435, 437, 9, 2,
	2, 2, 436, 198, 3, 2, 2, 2, 436, 211, 3, 2, 2, 2, 436, 224, 3, 2, 2, 2,
	436, 237, 3, 2, 2, 2, 436, 262, 3, 2, 2, 2, 436, 287, 3, 2, 2, 2, 436,
	302, 3, 2, 2, 2, 436, 317, 3, 2, 2, 2, 436, 332, 3, 2, 2, 2, 436, 347,
	3, 2, 2, 2, 436, 362, 3, 2, 2, 2, 436, 377, 3, 2, 2, 2, 436, 392, 3, 2,
	2, 2, 436, 407, 3, 2, 2, 2, 436, 422, 3, 2, 2, 2, 437, 13, 3, 2, 2, 2,
	66, 18, 25, 32, 38, 44, 53, 60, 67, 73, 79, 88, 95, 102, 108, 114, 122,
	128, 134, 145, 152, 157, 163, 169, 174, 180, 186, 191, 193, 195, 202, 208,
	215, 221, 228, 234, 241, 247, 253, 259, 266, 272, 278, 284, 291, 298, 306,
	313, 321, 328, 336, 343, 351, 358, 366, 373, 381, 388, 396, 403, 411, 418,
	426, 433, 436,
}
var deserializer = antlr.NewATNDeserializer(nil)
var deserializedATN = deserializer.DeserializeFromUInt16(parserATN)

var literalNames = []string{
	"", "','", "", "'('", "')'", "'['", "']'",
}
var symbolicNames = []string{
	"", "", "WS", "LPAREN", "RPAREN", "LBRACKET", "RBRACKET", "AND", "OR",
	"LT", "GT", "EQ", "CONTAINS", "IN", "BETWEEN", "BOOL", "DATETIME", "STRING",
	"NUMBER", "NULL", "IDENTIFIER", "RFC3339_DATE_TIME",
}

var ruleNames = []string{
	"string_array", "number_array", "datetime_array", "start", "expression",
	"operation",
}
var decisionToDFA = make([]*antlr.DFA, len(deserializedATN.DecisionToState))

func init() {
	for index, ds := range deserializedATN.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(ds, index)
	}
}

type ZitiQlParser struct {
	*antlr.BaseParser
}

func NewZitiQlParser(input antlr.TokenStream) *ZitiQlParser {
	this := new(ZitiQlParser)

	this.BaseParser = antlr.NewBaseParser(input)

	this.Interpreter = antlr.NewParserATNSimulator(this, deserializedATN, decisionToDFA, antlr.NewPredictionContextCache())
	this.RuleNames = ruleNames
	this.LiteralNames = literalNames
	this.SymbolicNames = symbolicNames
	this.GrammarFileName = "ZitiQl.g4"

	return this
}

// ZitiQlParser tokens.
const (
	ZitiQlParserEOF               = antlr.TokenEOF
	ZitiQlParserT__0              = 1
	ZitiQlParserWS                = 2
	ZitiQlParserLPAREN            = 3
	ZitiQlParserRPAREN            = 4
	ZitiQlParserLBRACKET          = 5
	ZitiQlParserRBRACKET          = 6
	ZitiQlParserAND               = 7
	ZitiQlParserOR                = 8
	ZitiQlParserLT                = 9
	ZitiQlParserGT                = 10
	ZitiQlParserEQ                = 11
	ZitiQlParserCONTAINS          = 12
	ZitiQlParserIN                = 13
	ZitiQlParserBETWEEN           = 14
	ZitiQlParserBOOL              = 15
	ZitiQlParserDATETIME          = 16
	ZitiQlParserSTRING            = 17
	ZitiQlParserNUMBER            = 18
	ZitiQlParserNULL              = 19
	ZitiQlParserIDENTIFIER        = 20
	ZitiQlParserRFC3339_DATE_TIME = 21
)

// ZitiQlParser rules.
const (
	ZitiQlParserRULE_string_array   = 0
	ZitiQlParserRULE_number_array   = 1
	ZitiQlParserRULE_datetime_array = 2
	ZitiQlParserRULE_start          = 3
	ZitiQlParserRULE_expression     = 4
	ZitiQlParserRULE_operation      = 5
)

// IString_arrayContext is an interface to support dynamic dispatch.
type IString_arrayContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsString_arrayContext differentiates from other interfaces.
	IsString_arrayContext()
}

type String_arrayContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyString_arrayContext() *String_arrayContext {
	var p = new(String_arrayContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = ZitiQlParserRULE_string_array
	return p
}

func (*String_arrayContext) IsString_arrayContext() {}

func NewString_arrayContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *String_arrayContext {
	var p = new(String_arrayContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_string_array

	return p
}

func (s *String_arrayContext) GetParser() antlr.Parser { return s.parser }

func (s *String_arrayContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLBRACKET, 0)
}

func (s *String_arrayContext) AllSTRING() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserSTRING)
}

func (s *String_arrayContext) STRING(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, i)
}

func (s *String_arrayContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRBRACKET, 0)
}

func (s *String_arrayContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *String_arrayContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *String_arrayContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *String_arrayContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *String_arrayContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterString_array(s)
	}
}

func (s *String_arrayContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitString_array(s)
	}
}

func (p *ZitiQlParser) String_array() (localctx IString_arrayContext) {
	localctx = NewString_arrayContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, ZitiQlParserRULE_string_array)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(12)
		p.Match(ZitiQlParserLBRACKET)
	}
	p.SetState(16)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(13)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(18)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(19)
		p.Match(ZitiQlParserSTRING)
	}
	p.SetState(36)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(23)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(20)
					p.Match(ZitiQlParserWS)
				}

				p.SetState(25)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(26)
				p.Match(ZitiQlParserT__0)
			}
			p.SetState(30)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(27)
					p.Match(ZitiQlParserWS)
				}

				p.SetState(32)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(33)
				p.Match(ZitiQlParserSTRING)
			}

		}
		p.SetState(38)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 3, p.GetParserRuleContext())
	}
	p.SetState(42)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(39)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(44)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(45)
		p.Match(ZitiQlParserRBRACKET)
	}

	return localctx
}

// INumber_arrayContext is an interface to support dynamic dispatch.
type INumber_arrayContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNumber_arrayContext differentiates from other interfaces.
	IsNumber_arrayContext()
}

type Number_arrayContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNumber_arrayContext() *Number_arrayContext {
	var p = new(Number_arrayContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = ZitiQlParserRULE_number_array
	return p
}

func (*Number_arrayContext) IsNumber_arrayContext() {}

func NewNumber_arrayContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Number_arrayContext {
	var p = new(Number_arrayContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_number_array

	return p
}

func (s *Number_arrayContext) GetParser() antlr.Parser { return s.parser }

func (s *Number_arrayContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLBRACKET, 0)
}

func (s *Number_arrayContext) AllNUMBER() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserNUMBER)
}

func (s *Number_arrayContext) NUMBER(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, i)
}

func (s *Number_arrayContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRBRACKET, 0)
}

func (s *Number_arrayContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *Number_arrayContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *Number_arrayContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Number_arrayContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Number_arrayContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterNumber_array(s)
	}
}

func (s *Number_arrayContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitNumber_array(s)
	}
}

func (p *ZitiQlParser) Number_array() (localctx INumber_arrayContext) {
	localctx = NewNumber_arrayContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, ZitiQlParserRULE_number_array)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(47)
		p.Match(ZitiQlParserLBRACKET)
	}
	p.SetState(51)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(48)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(53)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(54)
		p.Match(ZitiQlParserNUMBER)
	}
	p.SetState(71)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(58)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(55)
					p.Match(ZitiQlParserWS)
				}

				p.SetState(60)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(61)
				p.Match(ZitiQlParserT__0)
			}
			p.SetState(65)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(62)
					p.Match(ZitiQlParserWS)
				}

				p.SetState(67)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(68)
				p.Match(ZitiQlParserNUMBER)
			}

		}
		p.SetState(73)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext())
	}
	p.SetState(77)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(74)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(79)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(80)
		p.Match(ZitiQlParserRBRACKET)
	}

	return localctx
}

// IDatetime_arrayContext is an interface to support dynamic dispatch.
type IDatetime_arrayContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsDatetime_arrayContext differentiates from other interfaces.
	IsDatetime_arrayContext()
}

type Datetime_arrayContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDatetime_arrayContext() *Datetime_arrayContext {
	var p = new(Datetime_arrayContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = ZitiQlParserRULE_datetime_array
	return p
}

func (*Datetime_arrayContext) IsDatetime_arrayContext() {}

func NewDatetime_arrayContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Datetime_arrayContext {
	var p = new(Datetime_arrayContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_datetime_array

	return p
}

func (s *Datetime_arrayContext) GetParser() antlr.Parser { return s.parser }

func (s *Datetime_arrayContext) LBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLBRACKET, 0)
}

func (s *Datetime_arrayContext) AllDATETIME() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserDATETIME)
}

func (s *Datetime_arrayContext) DATETIME(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, i)
}

func (s *Datetime_arrayContext) RBRACKET() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRBRACKET, 0)
}

func (s *Datetime_arrayContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *Datetime_arrayContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *Datetime_arrayContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Datetime_arrayContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Datetime_arrayContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterDatetime_array(s)
	}
}

func (s *Datetime_arrayContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitDatetime_array(s)
	}
}

func (p *ZitiQlParser) Datetime_array() (localctx IDatetime_arrayContext) {
	localctx = NewDatetime_arrayContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, ZitiQlParserRULE_datetime_array)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(82)
		p.Match(ZitiQlParserLBRACKET)
	}
	p.SetState(86)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(83)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(88)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(89)
		p.Match(ZitiQlParserDATETIME)
	}
	p.SetState(106)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 13, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			p.SetState(93)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(90)
					p.Match(ZitiQlParserWS)
				}

				p.SetState(95)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(96)
				p.Match(ZitiQlParserT__0)
			}
			p.SetState(100)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)

			for _la == ZitiQlParserWS {
				{
					p.SetState(97)
					p.Match(ZitiQlParserWS)
				}

				p.SetState(102)
				p.GetErrorHandler().Sync(p)
				_la = p.GetTokenStream().LA(1)
			}
			{
				p.SetState(103)
				p.Match(ZitiQlParserDATETIME)
			}

		}
		p.SetState(108)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 13, p.GetParserRuleContext())
	}
	p.SetState(112)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(109)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(114)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(115)
		p.Match(ZitiQlParserRBRACKET)
	}

	return localctx
}

// IStartContext is an interface to support dynamic dispatch.
type IStartContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsStartContext differentiates from other interfaces.
	IsStartContext()
}

type StartContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStartContext() *StartContext {
	var p = new(StartContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = ZitiQlParserRULE_start
	return p
}

func (*StartContext) IsStartContext() {}

func NewStartContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StartContext {
	var p = new(StartContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_start

	return p
}

func (s *StartContext) GetParser() antlr.Parser { return s.parser }

func (s *StartContext) CopyFrom(ctx *StartContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *StartContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StartContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type EndContext struct {
	*StartContext
}

func NewEndContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *EndContext {
	var p = new(EndContext)

	p.StartContext = NewEmptyStartContext()
	p.parser = parser
	p.CopyFrom(ctx.(*StartContext))

	return p
}

func (s *EndContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *EndContext) EOF() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEOF, 0)
}

func (s *EndContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *EndContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *EndContext) AllExpression() []IExpressionContext {
	var ts = s.GetTypedRuleContexts(reflect.TypeOf((*IExpressionContext)(nil)).Elem())
	var tst = make([]IExpressionContext, len(ts))

	for i, t := range ts {
		if t != nil {
			tst[i] = t.(IExpressionContext)
		}
	}

	return tst
}

func (s *EndContext) Expression(i int) IExpressionContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IExpressionContext)(nil)).Elem(), i)

	if t == nil {
		return nil
	}

	return t.(IExpressionContext)
}

func (s *EndContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterEnd(s)
	}
}

func (s *EndContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitEnd(s)
	}
}

func (p *ZitiQlParser) Start() (localctx IStartContext) {
	localctx = NewStartContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, ZitiQlParserRULE_start)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	localctx = NewEndContext(p, localctx)
	p.EnterOuterAlt(localctx, 1)
	p.SetState(120)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(117)
				p.Match(ZitiQlParserWS)
			}

		}
		p.SetState(122)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 15, p.GetParserRuleContext())
	}
	p.SetState(126)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserLPAREN || _la == ZitiQlParserIDENTIFIER {
		{
			p.SetState(123)
			p.expression(0)
		}

		p.SetState(128)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	p.SetState(132)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == ZitiQlParserWS {
		{
			p.SetState(129)
			p.Match(ZitiQlParserWS)
		}

		p.SetState(134)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(135)
		p.Match(ZitiQlParserEOF)
	}

	return localctx
}

// IExpressionContext is an interface to support dynamic dispatch.
type IExpressionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsExpressionContext differentiates from other interfaces.
	IsExpressionContext()
}

type ExpressionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExpressionContext() *ExpressionContext {
	var p = new(ExpressionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = ZitiQlParserRULE_expression
	return p
}

func (*ExpressionContext) IsExpressionContext() {}

func NewExpressionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExpressionContext {
	var p = new(ExpressionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_expression

	return p
}

func (s *ExpressionContext) GetParser() antlr.Parser { return s.parser }

func (s *ExpressionContext) CopyFrom(ctx *ExpressionContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *ExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExpressionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type GroupContext struct {
	*ExpressionContext
}

func NewGroupContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *GroupContext {
	var p = new(GroupContext)

	p.ExpressionContext = NewEmptyExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExpressionContext))

	return p
}

func (s *GroupContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *GroupContext) LPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLPAREN, 0)
}

func (s *GroupContext) Expression() IExpressionContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IExpressionContext)(nil)).Elem(), 0)

	if t == nil {
		return nil
	}

	return t.(IExpressionContext)
}

func (s *GroupContext) RPAREN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserRPAREN, 0)
}

func (s *GroupContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *GroupContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *GroupContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterGroup(s)
	}
}

func (s *GroupContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitGroup(s)
	}
}

type OrConjunctionContext struct {
	*ExpressionContext
}

func NewOrConjunctionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OrConjunctionContext {
	var p = new(OrConjunctionContext)

	p.ExpressionContext = NewEmptyExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExpressionContext))

	return p
}

func (s *OrConjunctionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OrConjunctionContext) AllExpression() []IExpressionContext {
	var ts = s.GetTypedRuleContexts(reflect.TypeOf((*IExpressionContext)(nil)).Elem())
	var tst = make([]IExpressionContext, len(ts))

	for i, t := range ts {
		if t != nil {
			tst[i] = t.(IExpressionContext)
		}
	}

	return tst
}

func (s *OrConjunctionContext) Expression(i int) IExpressionContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IExpressionContext)(nil)).Elem(), i)

	if t == nil {
		return nil
	}

	return t.(IExpressionContext)
}

func (s *OrConjunctionContext) AllOR() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserOR)
}

func (s *OrConjunctionContext) OR(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserOR, i)
}

func (s *OrConjunctionContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *OrConjunctionContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *OrConjunctionContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterOrConjunction(s)
	}
}

func (s *OrConjunctionContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitOrConjunction(s)
	}
}

type OperationOpContext struct {
	*ExpressionContext
}

func NewOperationOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *OperationOpContext {
	var p = new(OperationOpContext)

	p.ExpressionContext = NewEmptyExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExpressionContext))

	return p
}

func (s *OperationOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperationOpContext) Operation() IOperationContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IOperationContext)(nil)).Elem(), 0)

	if t == nil {
		return nil
	}

	return t.(IOperationContext)
}

func (s *OperationOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterOperationOp(s)
	}
}

func (s *OperationOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitOperationOp(s)
	}
}

type AndConjunctionContext struct {
	*ExpressionContext
}

func NewAndConjunctionContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *AndConjunctionContext {
	var p = new(AndConjunctionContext)

	p.ExpressionContext = NewEmptyExpressionContext()
	p.parser = parser
	p.CopyFrom(ctx.(*ExpressionContext))

	return p
}

func (s *AndConjunctionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AndConjunctionContext) AllExpression() []IExpressionContext {
	var ts = s.GetTypedRuleContexts(reflect.TypeOf((*IExpressionContext)(nil)).Elem())
	var tst = make([]IExpressionContext, len(ts))

	for i, t := range ts {
		if t != nil {
			tst[i] = t.(IExpressionContext)
		}
	}

	return tst
}

func (s *AndConjunctionContext) Expression(i int) IExpressionContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IExpressionContext)(nil)).Elem(), i)

	if t == nil {
		return nil
	}

	return t.(IExpressionContext)
}

func (s *AndConjunctionContext) AllAND() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserAND)
}

func (s *AndConjunctionContext) AND(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserAND, i)
}

func (s *AndConjunctionContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *AndConjunctionContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *AndConjunctionContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterAndConjunction(s)
	}
}

func (s *AndConjunctionContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitAndConjunction(s)
	}
}

func (p *ZitiQlParser) Expression() (localctx IExpressionContext) {
	return p.expression(0)
}

func (p *ZitiQlParser) expression(_p int) (localctx IExpressionContext) {
	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewExpressionContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IExpressionContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 8
	p.EnterRecursionRule(localctx, 8, ZitiQlParserRULE_expression, _p)
	var _la int

	defer func() {
		p.UnrollRecursionContexts(_parentctx)
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(155)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case ZitiQlParserIDENTIFIER:
		localctx = NewOperationOpContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx

		{
			p.SetState(138)
			p.Operation()
		}

	case ZitiQlParserLPAREN:
		localctx = NewGroupContext(p, localctx)
		p.SetParserRuleContext(localctx)
		_prevctx = localctx
		{
			p.SetState(139)
			p.Match(ZitiQlParserLPAREN)
		}
		p.SetState(143)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(140)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(145)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(146)
			p.expression(0)
		}
		p.SetState(150)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(147)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(152)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(153)
			p.Match(ZitiQlParserRPAREN)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}
	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(193)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 28, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			p.SetState(191)
			p.GetErrorHandler().Sync(p)
			switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 27, p.GetParserRuleContext()) {
			case 1:
				localctx = NewAndConjunctionContext(p, NewExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, ZitiQlParserRULE_expression)
				p.SetState(157)

				if !(p.Precpred(p.GetParserRuleContext(), 2)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 2)", ""))
				}
				p.SetState(170)
				p.GetErrorHandler().Sync(p)
				_alt = 1
				for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
					switch _alt {
					case 1:
						p.SetState(159)
						p.GetErrorHandler().Sync(p)
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(158)
								p.Match(ZitiQlParserWS)
							}

							p.SetState(161)
							p.GetErrorHandler().Sync(p)
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(163)
							p.Match(ZitiQlParserAND)
						}
						p.SetState(165)
						p.GetErrorHandler().Sync(p)
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(164)
								p.Match(ZitiQlParserWS)
							}

							p.SetState(167)
							p.GetErrorHandler().Sync(p)
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(169)
							p.expression(0)
						}

					default:
						panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
					}

					p.SetState(172)
					p.GetErrorHandler().Sync(p)
					_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 23, p.GetParserRuleContext())
				}

			case 2:
				localctx = NewOrConjunctionContext(p, NewExpressionContext(p, _parentctx, _parentState))
				p.PushNewRecursionContext(localctx, _startState, ZitiQlParserRULE_expression)
				p.SetState(174)

				if !(p.Precpred(p.GetParserRuleContext(), 1)) {
					panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
				}
				p.SetState(187)
				p.GetErrorHandler().Sync(p)
				_alt = 1
				for ok := true; ok; ok = _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
					switch _alt {
					case 1:
						p.SetState(176)
						p.GetErrorHandler().Sync(p)
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(175)
								p.Match(ZitiQlParserWS)
							}

							p.SetState(178)
							p.GetErrorHandler().Sync(p)
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(180)
							p.Match(ZitiQlParserOR)
						}
						p.SetState(182)
						p.GetErrorHandler().Sync(p)
						_la = p.GetTokenStream().LA(1)

						for ok := true; ok; ok = _la == ZitiQlParserWS {
							{
								p.SetState(181)
								p.Match(ZitiQlParserWS)
							}

							p.SetState(184)
							p.GetErrorHandler().Sync(p)
							_la = p.GetTokenStream().LA(1)
						}
						{
							p.SetState(186)
							p.expression(0)
						}

					default:
						panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
					}

					p.SetState(189)
					p.GetErrorHandler().Sync(p)
					_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 26, p.GetParserRuleContext())
				}

			}

		}
		p.SetState(195)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 28, p.GetParserRuleContext())
	}

	return localctx
}

// IOperationContext is an interface to support dynamic dispatch.
type IOperationContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsOperationContext differentiates from other interfaces.
	IsOperationContext()
}

type OperationContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyOperationContext() *OperationContext {
	var p = new(OperationContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = ZitiQlParserRULE_operation
	return p
}

func (*OperationContext) IsOperationContext() {}

func NewOperationContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *OperationContext {
	var p = new(OperationContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = ZitiQlParserRULE_operation

	return p
}

func (s *OperationContext) GetParser() antlr.Parser { return s.parser }

func (s *OperationContext) CopyFrom(ctx *OperationContext) {
	s.BaseParserRuleContext.CopyFrom(ctx.BaseParserRuleContext)
}

func (s *OperationContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *OperationContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

type BinaryEqualToNullOpContext struct {
	*OperationContext
}

func NewBinaryEqualToNullOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToNullOpContext {
	var p = new(BinaryEqualToNullOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToNullOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToNullOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryEqualToNullOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToNullOpContext) NULL() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNULL, 0)
}

func (s *BinaryEqualToNullOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToNullOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToNullOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToNullOp(s)
	}
}

func (s *BinaryEqualToNullOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToNullOp(s)
	}
}

type BinaryLessThanNumberOpContext struct {
	*OperationContext
}

func NewBinaryLessThanNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryLessThanNumberOpContext {
	var p = new(BinaryLessThanNumberOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryLessThanNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryLessThanNumberOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryLessThanNumberOpContext) LT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLT, 0)
}

func (s *BinaryLessThanNumberOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryLessThanNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryLessThanNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryLessThanNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryLessThanNumberOp(s)
	}
}

func (s *BinaryLessThanNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryLessThanNumberOp(s)
	}
}

type BinaryGreaterThanDatetimeOpContext struct {
	*OperationContext
}

func NewBinaryGreaterThanDatetimeOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryGreaterThanDatetimeOpContext {
	var p = new(BinaryGreaterThanDatetimeOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryGreaterThanDatetimeOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryGreaterThanDatetimeOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryGreaterThanDatetimeOpContext) GT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserGT, 0)
}

func (s *BinaryGreaterThanDatetimeOpContext) DATETIME() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, 0)
}

func (s *BinaryGreaterThanDatetimeOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryGreaterThanDatetimeOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryGreaterThanDatetimeOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryGreaterThanDatetimeOp(s)
	}
}

func (s *BinaryGreaterThanDatetimeOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryGreaterThanDatetimeOp(s)
	}
}

type InNumberArrayOpContext struct {
	*OperationContext
}

func NewInNumberArrayOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InNumberArrayOpContext {
	var p = new(InNumberArrayOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *InNumberArrayOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InNumberArrayOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *InNumberArrayOpContext) IN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIN, 0)
}

func (s *InNumberArrayOpContext) Number_array() INumber_arrayContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*INumber_arrayContext)(nil)).Elem(), 0)

	if t == nil {
		return nil
	}

	return t.(INumber_arrayContext)
}

func (s *InNumberArrayOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *InNumberArrayOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *InNumberArrayOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterInNumberArrayOp(s)
	}
}

func (s *InNumberArrayOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitInNumberArrayOp(s)
	}
}

type InStringArrayOpContext struct {
	*OperationContext
}

func NewInStringArrayOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InStringArrayOpContext {
	var p = new(InStringArrayOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *InStringArrayOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InStringArrayOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *InStringArrayOpContext) IN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIN, 0)
}

func (s *InStringArrayOpContext) String_array() IString_arrayContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IString_arrayContext)(nil)).Elem(), 0)

	if t == nil {
		return nil
	}

	return t.(IString_arrayContext)
}

func (s *InStringArrayOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *InStringArrayOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *InStringArrayOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterInStringArrayOp(s)
	}
}

func (s *InStringArrayOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitInStringArrayOp(s)
	}
}

type BinaryLessThanDatetimeOpContext struct {
	*OperationContext
}

func NewBinaryLessThanDatetimeOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryLessThanDatetimeOpContext {
	var p = new(BinaryLessThanDatetimeOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryLessThanDatetimeOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryLessThanDatetimeOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryLessThanDatetimeOpContext) LT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserLT, 0)
}

func (s *BinaryLessThanDatetimeOpContext) DATETIME() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, 0)
}

func (s *BinaryLessThanDatetimeOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryLessThanDatetimeOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryLessThanDatetimeOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryLessThanDatetimeOp(s)
	}
}

func (s *BinaryLessThanDatetimeOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryLessThanDatetimeOp(s)
	}
}

type BinaryGreaterThanNumberOpContext struct {
	*OperationContext
}

func NewBinaryGreaterThanNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryGreaterThanNumberOpContext {
	var p = new(BinaryGreaterThanNumberOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryGreaterThanNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryGreaterThanNumberOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryGreaterThanNumberOpContext) GT() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserGT, 0)
}

func (s *BinaryGreaterThanNumberOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryGreaterThanNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryGreaterThanNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryGreaterThanNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryGreaterThanNumberOp(s)
	}
}

func (s *BinaryGreaterThanNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryGreaterThanNumberOp(s)
	}
}

type InDatetimeArrayOpContext struct {
	*OperationContext
}

func NewInDatetimeArrayOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *InDatetimeArrayOpContext {
	var p = new(InDatetimeArrayOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *InDatetimeArrayOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *InDatetimeArrayOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *InDatetimeArrayOpContext) IN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIN, 0)
}

func (s *InDatetimeArrayOpContext) Datetime_array() IDatetime_arrayContext {
	var t = s.GetTypedRuleContext(reflect.TypeOf((*IDatetime_arrayContext)(nil)).Elem(), 0)

	if t == nil {
		return nil
	}

	return t.(IDatetime_arrayContext)
}

func (s *InDatetimeArrayOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *InDatetimeArrayOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *InDatetimeArrayOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterInDatetimeArrayOp(s)
	}
}

func (s *InDatetimeArrayOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitInDatetimeArrayOp(s)
	}
}

type BetweenDateOpContext struct {
	*OperationContext
}

func NewBetweenDateOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BetweenDateOpContext {
	var p = new(BetweenDateOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BetweenDateOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BetweenDateOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BetweenDateOpContext) BETWEEN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBETWEEN, 0)
}

func (s *BetweenDateOpContext) AllDATETIME() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserDATETIME)
}

func (s *BetweenDateOpContext) DATETIME(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, i)
}

func (s *BetweenDateOpContext) AND() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserAND, 0)
}

func (s *BetweenDateOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BetweenDateOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BetweenDateOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBetweenDateOp(s)
	}
}

func (s *BetweenDateOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBetweenDateOp(s)
	}
}

type BinaryEqualToNumberOpContext struct {
	*OperationContext
}

func NewBinaryEqualToNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToNumberOpContext {
	var p = new(BinaryEqualToNumberOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToNumberOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryEqualToNumberOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToNumberOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryEqualToNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToNumberOp(s)
	}
}

func (s *BinaryEqualToNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToNumberOp(s)
	}
}

type BinaryEqualToBoolOpContext struct {
	*OperationContext
}

func NewBinaryEqualToBoolOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToBoolOpContext {
	var p = new(BinaryEqualToBoolOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToBoolOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToBoolOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryEqualToBoolOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToBoolOpContext) BOOL() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBOOL, 0)
}

func (s *BinaryEqualToBoolOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToBoolOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToBoolOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToBoolOp(s)
	}
}

func (s *BinaryEqualToBoolOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToBoolOp(s)
	}
}

type BinaryEqualToStringOpContext struct {
	*OperationContext
}

func NewBinaryEqualToStringOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToStringOpContext {
	var p = new(BinaryEqualToStringOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToStringOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToStringOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryEqualToStringOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToStringOpContext) STRING() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, 0)
}

func (s *BinaryEqualToStringOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToStringOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToStringOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToStringOp(s)
	}
}

func (s *BinaryEqualToStringOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToStringOp(s)
	}
}

type BetweenNumberOpContext struct {
	*OperationContext
}

func NewBetweenNumberOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BetweenNumberOpContext {
	var p = new(BetweenNumberOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BetweenNumberOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BetweenNumberOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BetweenNumberOpContext) BETWEEN() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserBETWEEN, 0)
}

func (s *BetweenNumberOpContext) AllNUMBER() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserNUMBER)
}

func (s *BetweenNumberOpContext) NUMBER(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, i)
}

func (s *BetweenNumberOpContext) AND() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserAND, 0)
}

func (s *BetweenNumberOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BetweenNumberOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BetweenNumberOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBetweenNumberOp(s)
	}
}

func (s *BetweenNumberOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBetweenNumberOp(s)
	}
}

type BinaryContainsOpContext struct {
	*OperationContext
}

func NewBinaryContainsOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryContainsOpContext {
	var p = new(BinaryContainsOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryContainsOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryContainsOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryContainsOpContext) CONTAINS() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserCONTAINS, 0)
}

func (s *BinaryContainsOpContext) STRING() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserSTRING, 0)
}

func (s *BinaryContainsOpContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserNUMBER, 0)
}

func (s *BinaryContainsOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryContainsOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryContainsOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryContainsOp(s)
	}
}

func (s *BinaryContainsOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryContainsOp(s)
	}
}

type BinaryEqualToDatetimeOpContext struct {
	*OperationContext
}

func NewBinaryEqualToDatetimeOpContext(parser antlr.Parser, ctx antlr.ParserRuleContext) *BinaryEqualToDatetimeOpContext {
	var p = new(BinaryEqualToDatetimeOpContext)

	p.OperationContext = NewEmptyOperationContext()
	p.parser = parser
	p.CopyFrom(ctx.(*OperationContext))

	return p
}

func (s *BinaryEqualToDatetimeOpContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BinaryEqualToDatetimeOpContext) IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserIDENTIFIER, 0)
}

func (s *BinaryEqualToDatetimeOpContext) EQ() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserEQ, 0)
}

func (s *BinaryEqualToDatetimeOpContext) DATETIME() antlr.TerminalNode {
	return s.GetToken(ZitiQlParserDATETIME, 0)
}

func (s *BinaryEqualToDatetimeOpContext) AllWS() []antlr.TerminalNode {
	return s.GetTokens(ZitiQlParserWS)
}

func (s *BinaryEqualToDatetimeOpContext) WS(i int) antlr.TerminalNode {
	return s.GetToken(ZitiQlParserWS, i)
}

func (s *BinaryEqualToDatetimeOpContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.EnterBinaryEqualToDatetimeOp(s)
	}
}

func (s *BinaryEqualToDatetimeOpContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ZitiQlListener); ok {
		listenerT.ExitBinaryEqualToDatetimeOp(s)
	}
}

func (p *ZitiQlParser) Operation() (localctx IOperationContext) {
	localctx = NewOperationContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, ZitiQlParserRULE_operation)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(434)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 63, p.GetParserRuleContext()) {
	case 1:
		localctx = NewInStringArrayOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(196)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(198)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(197)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(200)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(202)
			p.Match(ZitiQlParserIN)
		}
		p.SetState(204)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(203)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(206)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(208)
			p.String_array()
		}

	case 2:
		localctx = NewInNumberArrayOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(209)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(211)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(210)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(213)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(215)
			p.Match(ZitiQlParserIN)
		}
		p.SetState(217)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(216)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(219)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(221)
			p.Number_array()
		}

	case 3:
		localctx = NewInDatetimeArrayOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(222)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(224)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(223)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(226)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(228)
			p.Match(ZitiQlParserIN)
		}
		p.SetState(230)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(229)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(232)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(234)
			p.Datetime_array()
		}

	case 4:
		localctx = NewBetweenNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(235)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(237)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(236)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(239)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(241)
			p.Match(ZitiQlParserBETWEEN)
		}
		p.SetState(243)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(242)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(245)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(247)
			p.Match(ZitiQlParserNUMBER)
		}
		p.SetState(249)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(248)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(251)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(253)
			p.Match(ZitiQlParserAND)
		}
		p.SetState(255)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(254)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(257)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(259)
			p.Match(ZitiQlParserNUMBER)
		}

	case 5:
		localctx = NewBetweenDateOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(260)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(262)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(261)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(264)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(266)
			p.Match(ZitiQlParserBETWEEN)
		}
		p.SetState(268)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(267)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(270)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(272)
			p.Match(ZitiQlParserDATETIME)
		}
		p.SetState(274)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(273)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(276)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(278)
			p.Match(ZitiQlParserAND)
		}
		p.SetState(280)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(279)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(282)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(284)
			p.Match(ZitiQlParserDATETIME)
		}

	case 6:
		localctx = NewBinaryLessThanNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(285)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(289)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(286)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(291)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(292)
			p.Match(ZitiQlParserLT)
		}
		p.SetState(296)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(293)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(298)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(299)
			p.Match(ZitiQlParserNUMBER)
		}

	case 7:
		localctx = NewBinaryLessThanDatetimeOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(300)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(304)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(301)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(306)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(307)
			p.Match(ZitiQlParserLT)
		}
		p.SetState(311)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(308)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(313)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(314)
			p.Match(ZitiQlParserDATETIME)
		}

	case 8:
		localctx = NewBinaryGreaterThanNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(315)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(319)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(316)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(321)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(322)
			p.Match(ZitiQlParserGT)
		}
		p.SetState(326)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(323)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(328)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(329)
			p.Match(ZitiQlParserNUMBER)
		}

	case 9:
		localctx = NewBinaryGreaterThanDatetimeOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(330)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(334)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(331)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(336)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(337)
			p.Match(ZitiQlParserGT)
		}
		p.SetState(341)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(338)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(343)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(344)
			p.Match(ZitiQlParserDATETIME)
		}

	case 10:
		localctx = NewBinaryEqualToStringOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(345)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(349)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(346)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(351)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(352)
			p.Match(ZitiQlParserEQ)
		}
		p.SetState(356)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(353)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(358)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(359)
			p.Match(ZitiQlParserSTRING)
		}

	case 11:
		localctx = NewBinaryEqualToNumberOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(360)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(364)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(361)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(366)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(367)
			p.Match(ZitiQlParserEQ)
		}
		p.SetState(371)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(368)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(373)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(374)
			p.Match(ZitiQlParserNUMBER)
		}

	case 12:
		localctx = NewBinaryEqualToDatetimeOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(375)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(379)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(376)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(381)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(382)
			p.Match(ZitiQlParserEQ)
		}
		p.SetState(386)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(383)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(388)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(389)
			p.Match(ZitiQlParserDATETIME)
		}

	case 13:
		localctx = NewBinaryEqualToBoolOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(390)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(394)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(391)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(396)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(397)
			p.Match(ZitiQlParserEQ)
		}
		p.SetState(401)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(398)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(403)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(404)
			p.Match(ZitiQlParserBOOL)
		}

	case 14:
		localctx = NewBinaryEqualToNullOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 14)
		{
			p.SetState(405)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(409)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(406)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(411)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(412)
			p.Match(ZitiQlParserEQ)
		}
		p.SetState(416)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(413)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(418)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(419)
			p.Match(ZitiQlParserNULL)
		}

	case 15:
		localctx = NewBinaryContainsOpContext(p, localctx)
		p.EnterOuterAlt(localctx, 15)
		{
			p.SetState(420)
			p.Match(ZitiQlParserIDENTIFIER)
		}
		p.SetState(424)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == ZitiQlParserWS {
			{
				p.SetState(421)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(426)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(427)
			p.Match(ZitiQlParserCONTAINS)
		}
		p.SetState(429)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for ok := true; ok; ok = _la == ZitiQlParserWS {
			{
				p.SetState(428)
				p.Match(ZitiQlParserWS)
			}

			p.SetState(431)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(433)
			_la = p.GetTokenStream().LA(1)

			if !(_la == ZitiQlParserSTRING || _la == ZitiQlParserNUMBER) {
				p.GetErrorHandler().RecoverInline(p)
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}

	}

	return localctx
}

func (p *ZitiQlParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 4:
		var t *ExpressionContext = nil
		if localctx != nil {
			t = localctx.(*ExpressionContext)
		}
		return p.Expression_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *ZitiQlParser) Expression_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 2)

	case 1:
		return p.Precpred(p.GetParserRuleContext(), 1)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
