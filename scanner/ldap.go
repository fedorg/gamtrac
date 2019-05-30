package scanner

import (
	"crypto/tls"
	"fmt"
	"gopkg.in/ldap.v2"
	"net"
	"reflect"
	"encoding/binary"
	"strconv"
	"strings"
)

const _PAGING_SIZE = 500

type LdapUserInfo struct {
	ObjectSid      string
	SAMAccountName string
	CN             string
	MemberOf       [][]string
	// sAMAccountType    string
	// userPrincipalName string
	// displayName       string
	// givenName         string
	// description       string
	// adminCount        string
	// homeDirectory     string
}

// LdapInfo contains connection info
type LdapInfo struct {
	LdapServer  string
	LdapIP      string
	LdapPort    uint16
	LdapTLSPort uint16
	User        string
	Usergpp     string
	Pass        string
	Domain      string
	Unsafe      bool
	StartTLS    bool
}

func dial(li *LdapInfo) (*ldap.Conn, error) {
	if li.Unsafe {
		fmt.Printf("[i] Begin PLAINTEXT LDAP connection to '%s' (%s)...\n", li.LdapServer, li.LdapIP)
		conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", li.LdapServer, li.LdapPort))
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] PLAINTEXT LDAP connection to '%s' (%s) successful...\n", li.LdapServer, li.LdapIP)
		return conn, nil
	} else if li.StartTLS {
		fmt.Printf("[i] Begin PLAINTEXT LDAP connection to '%s' (%s)...\n", li.LdapServer, li.LdapIP)
		conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", li.LdapServer, li.LdapPort))
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] PLAINTEXT LDAP connection to '%s' (%s) successful...\n[i] Upgrade to StartTLS connection...\n", li.LdapServer, li.LdapIP)
		err = conn.StartTLS(&tls.Config{ServerName: li.LdapServer})
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] Upgrade to StartTLS connection successful...\n")
		return conn, nil
	} else {
		fmt.Printf("[i] Begin LDAP TLS connection to '%s' (%s)...\n", li.LdapServer, li.LdapIP)
		config := &tls.Config{ServerName: li.LdapServer}
		conn, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", li.LdapServer, li.LdapTLSPort), config)
		if err != nil {
			return nil, err
		}
		fmt.Printf("[i] LDAP TLS connection to '%s' (%s) successful...\n", li.LdapServer, li.LdapIP)
		return conn, nil
	}
}

// Connect authenticated bind to ldap connection
func LdapConnect(li *LdapInfo) (*ldap.Conn, error) {
	conn, err := dial(li)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[i] Begin BIND...\n")
	err = conn.Bind(li.User, li.Pass)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[i] BIND with '%s' successful...\n", li.User)
	return conn, nil
}

func NewConnectionInfo(ldapServer string, domain string, user string, pass string, unsafe bool, tls bool) (*LdapInfo, error) {
	addr, err := net.LookupIP(ldapServer)
	if err != nil {
		return nil, err
	}
	ldapIP := addr[0].String()

	li := &LdapInfo{
		LdapServer:  ldapServer,
		LdapIP:      ldapIP,
		LdapPort:    uint16(389),
		LdapTLSPort: uint16(636),
		User:        user + "@" + domain,
		Usergpp:     user,
		Pass:        pass,
		Domain:      domain,
		Unsafe:      unsafe,
		StartTLS:    tls}
	return li, nil
}

func GetStructFields(t interface{}) []string {
	s := reflect.ValueOf(t).Elem()
	typeOfT := s.Type()
	ret := []string{}
	for i := 0; i < s.NumField(); i++ {
		// f := s.Field(i)
		ret = append(ret, typeOfT.Field(i).Name)
	}
	return ret
}

func SetStructStringField(t interface{}, field string, value string) error {
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("decode requires non-nil pointer")
	}
	v = v.Elem()
	f := v.FieldByName(field)
	if !f.IsValid() || !f.CanSet() {
		return fmt.Errorf("Could not set field %v", field)
	}
	if f.Kind() != reflect.String || f.String() != "" {
		return fmt.Errorf("Field %v is not string", field)
	}
	f.SetString(value)
	return nil
}

// Helper function for LDAP search
func LdapSearch(searchDN string, filter string, attributes []string, conn *ldap.Conn) (*ldap.SearchResult, error) {
	searchRequest := ldap.NewSearchRequest(
		searchDN,
		ldap.ScopeWholeSubtree, ldap.DerefAlways, 0, 0, false,
		filter,
		attributes,
		nil)

	sr, err := conn.SearchWithPaging(searchRequest, _PAGING_SIZE)
	if err != nil {
		return nil, err
	}
	return sr, nil
}

type SID []byte

func parseGroup(dn string) ([]string, error) {
	dnParsed, err := ldap.ParseDN(dn)
	if err != nil {
		return nil, err
	}
	grps := []string{}
	for _, rdn := range  dnParsed.RDNs {
		for _, tv := range rdn.Attributes {
			grps = append(grps, tv.Type + "=" + tv.Value)
		}
	}
	if (len(grps) == 0) {
		return nil, fmt.Errorf("dn `%v` does not contain group description", dn)
	}
	// hierarchical group order
	rgrp := make([]string, len(grps))
	for i, g := range grps {
		rgrp[len(grps)-1-i] = g
	}
	return rgrp, nil
}


func FilterGroups(memberships [][]string, required []string) [][]string {
	ret := [][]string{}
	for _, grp := range memberships {
		if len(required) > len(grp) {
			continue
		}
		passed := true
		for i, rq := range required {
			if grp[i] != rq {
				passed = false
				break
			}
		}
		if !passed {
			continue
		}
		ret = append(ret, grp[len(required):])
	}
	return ret
}

// func LdapSearchUsers(conn *ldap.Conn, searchDN string, filter string) ([], error) {
// 	//the namespace objects are located in the container CN=Dfs-Configuration,CN=System,DC=domain,DC=fqdn, and are of the objectClass msDFS-Namespacev2
// 	// (name=$_)
// 	// LDAPFilter = '(&(objectClass=msDFS-Namespacev2){0})' -f $NamespaceClause
//     // SearchBase = 'CN=Dfs-Configuration,{0}' -f $SystemDN
// }

func LdapSearchUsers(conn *ldap.Conn, searchDN string, filter string) ([]LdapUserInfo, error) {
	if filter == "" {
		filter = "(&(objectCategory=person)(objectClass=user)(SamAccountName=*))"
	}
	attributes := GetStructFields(&LdapUserInfo{})
	sr, err := LdapSearch(searchDN, filter, attributes, conn)
	if err != nil {
		return nil, err
	}

	ret := []LdapUserInfo{}
	// case-insensitive get
	getAttributes := func(e *ldap.Entry, attribute string) []string {
		for _, attr := range e.Attributes {
			if strings.ToLower(attr.Name) == strings.ToLower(attribute) {
				if len(attr.Values) == 0 {
					return []string{""}
				}
				return attr.Values
			}
		}
		return []string{""}
	}
	for _, entry := range sr.Entries {
		// mem := strings.Join(entry.GetAttributeValues("memberOf"), " ")
		info := new(LdapUserInfo)
		for _, fld := range GetStructFields(info) {
			val := strings.Join(getAttributes(entry, fld), "\n")
			fldlow := strings.ToLower(fld)
			switch fldlow{
			case "objectsid":
				val = SID(val).String()
			case "memberof":
				memberships := [][]string{}
				for _, dn := range getAttributes(entry, fld) {
					if dn == "" {continue}
					grp, err := parseGroup(dn)
					if err != nil {
						fmt.Println(err)
						continue
					}
					memberships = append(memberships, grp)
				}
				info.MemberOf = memberships
				
				
				continue

			}
			err := SetStructStringField(info, fld, val)
			if err != nil {
				return nil, err
			}
		}
		ret = append(ret, *info)
	}
	return ret, nil
}


func (s SID) String() string {
	sidRevision := byte(1)
	if len(s) < 8 || s[0] != sidRevision || len(s) != (int(s[1])*4)+8 {
		return ""
	}

	ret := []byte("S-1-")
	ret = strconv.AppendUint(ret, binary.BigEndian.Uint64(s[:8])&0xFFFFFFFFFFFF, 10)

	for i := 0; i < int(s[1]); i++ {
		ret = append(ret, "-"...)
		ret = strconv.AppendUint(ret, uint64(binary.LittleEndian.Uint32(s[8+i*4:])), 10)
	}

	return string(ret)
}
